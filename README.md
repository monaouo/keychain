# Keychain

本機執行的帳號密碼管理 Web 應用。單一執行檔、無外部依賴，資料以主密碼加密後儲存於本機。

## 功能

- 帳號資料管理：標題、帳號、密碼、網址、標籤、備註
- 即時搜尋與標籤篩選
- 一鍵複製帳號／密碼／網址，30 秒後自動清除剪貼簿
- 密碼產生器（可調長度、符號）與強度提示
- 更換主密碼
- 閒置自動上鎖
- 淺色／深色主題、手機版版面

## 快速開始

需求：Go 1.26 以上。

```sh
make run          # 啟動於 http://127.0.0.1:8787
make build        # 產出 bin/keychain
```

首次開啟會要求設定主密碼（至少 8 個字元）。**主密碼遺失後資料無法復原**，請妥善保管。

### 參數

設定優先順序：命令列參數 > 環境變數 > 預設值。

| 參數 | 環境變數 | 預設值 | 說明 |
| --- | --- | --- | --- |
| `-addr` | `KEYCHAIN_ADDR` | `127.0.0.1:8787` | 監聽位址 |
| `-data` | `KEYCHAIN_DATA` | `~/.keychain/vault.json` | 保險庫檔案路徑 |
| `-idle` | `KEYCHAIN_IDLE` | `10m` | 閒置多久後自動上鎖 |
| `-secure-cookie` | `KEYCHAIN_SECURE_COOKIE` | `false` | cookie 加上 `Secure`，僅在 HTTPS 反向代理後啟用 |

```sh
./bin/keychain -data /path/to/vault.json -idle 5m
```

### 健康檢查

```sh
./bin/keychain healthcheck   # 服務正常時 exit 0，否則 exit 1
```

依相同設定取得位址；監聽 `0.0.0.0` 時會改連 `127.0.0.1`。

## 使用 Docker 部署

映像：`ghcr.io/monaouo/keychain`（支援 `linux/amd64`、`linux/arm64`）

### docker run

```sh
docker run -d --name keychain \
  -p 127.0.0.1:8787:8787 \
  -v keychain-data:/data \
  --restart unless-stopped \
  ghcr.io/monaouo/keychain:latest
```

開啟 http://127.0.0.1:8787 設定主密碼即可使用。

### docker compose

下載 [`docker-compose.yml`](docker-compose.yml) 後執行：

```sh
docker compose up -d
```

### 注意事項

- 埠號對應請保留 `127.0.0.1:` 前綴，只讓本機連線；若拿掉會讓區域網路內的其他裝置也能連到。
- 資料存放在 volume `keychain-data` 的 `/data/vault.json`。備份：
  ```sh
  docker cp keychain:/data/vault.json ./vault-backup.json
  ```
- 容器以非 root（UID 65532）執行。若改用主機目錄掛載，需先調整權限：
  ```sh
  mkdir -p ./data && sudo chown 65532:65532 ./data
  docker run -d -p 127.0.0.1:8787:8787 -v "$PWD/data:/data" ghcr.io/monaouo/keychain:latest
  ```
- 放在 HTTPS 反向代理之後時，加上 `-e KEYCHAIN_SECURE_COOKIE=true`。
- 升級：`docker compose pull && docker compose up -d`，資料保留在 volume 中。

### 發佈映像（維護者）

1. 到 GitHub → Settings → Developer settings → Personal access tokens (classic) 建立 token，勾選 `write:packages`。
2. 登入 GHCR：
   ```sh
   echo <TOKEN> | docker login ghcr.io -u monaouo --password-stdin
   ```
3. 首次使用 buildx 多架構建置需建立 builder：
   ```sh
   docker buildx create --name keychain-builder --use
   ```
4. 建置並推送：
   ```sh
   make docker-push VERSION=0.1.0
   ```
5. GHCR 新套件預設為私有：到 GitHub 個人頁 → Packages → keychain → Package settings → Change visibility 改為 **Public**，他人才能免登入拉取。

本機測試映像：`make docker-build && make docker-run`。

## 安全設計

| 項目 | 做法 |
| --- | --- |
| 金鑰衍生 | PBKDF2-HMAC-SHA256，600,000 次迭代，16 位元組隨機 salt（建立或更換主密碼時產生） |
| 加密 | AES-256-GCM，整個保險庫一次加密，每次寫入使用新 nonce |
| 檔案寫入 | 暫存檔 + rename 原子寫入，權限 `0600` |
| 工作階段 | 32 位元組隨機 token，`HttpOnly` + `SameSite=Strict` cookie |
| CSRF | 所有非 GET 請求必須帶 `X-Keychain: 1` 標頭 |
| 暴力破解 | 解鎖序列化處理，失敗延遲 1 秒 |
| 記憶體 | 上鎖、逾時、關閉程式時清除金鑰 |
| 瀏覽器 | 嚴格 CSP（僅 `self`）、`X-Frame-Options: DENY`、`no-store` |

> 預設只監聽 `127.0.0.1`。若要讓其他裝置存取，請放在 HTTPS 反向代理之後，不要直接以 HTTP 對外公開。

### 保險庫檔案格式

```json
{
  "version": 1,
  "salt": "<base64>",
  "iter": 600000,
  "nonce": "<base64>",
  "data": "<base64 AES-GCM 密文>"
}
```

備份時直接複製此檔即可，沒有主密碼無法解開。

## API

除 `status` 外，變更類請求須帶 `X-Keychain: 1` 與 `Content-Type: application/json`。

| 方法 | 路徑 | 說明 |
| --- | --- | --- |
| GET | `/api/status` | `{initialized, unlocked}` |
| POST | `/api/setup` | `{password}` 建立保險庫 |
| POST | `/api/unlock` | `{password}` 解鎖 |
| POST | `/api/lock` | 上鎖 |
| GET | `/api/entries?q=` | 列出／搜尋 |
| POST | `/api/entries` | 新增 |
| GET | `/api/entries/{id}` | 取得單筆 |
| PUT | `/api/entries/{id}` | 更新 |
| DELETE | `/api/entries/{id}` | 刪除 |
| POST | `/api/password` | `{old, new}` 更換主密碼 |

## 專案結構

```
cmd/keychain/       程式進入點
internal/config/    設定載入（參數、環境變數）
internal/vault/     加密保險庫（金鑰衍生、加解密、CRUD）
internal/server/    HTTP API、工作階段、安全標頭
internal/web/       內嵌前端（HTML / CSS / JS）
test/               測試（不納入版本控制）
```

## 開發

```sh
make test   # go test -race ./...
make all    # fmt + vet + test + build
```

測試集中於 `test/`，已列入 `.gitignore`，僅存於本機。
