'use strict';

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const ICONS = {
  key: '<circle cx="8" cy="15" r="4"/><path d="M10.8 12.2 20 3M17 6l3 3M14 9l2 2"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  lock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
  eye: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
  eyeOff: '<path d="m3 3 18 18"/><path d="M10.6 5.1A9.8 9.8 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4.2M6.6 6.6A17 17 0 0 0 2 12s3.5 7 10 7a9.7 9.7 0 0 0 5.4-1.6"/><path d="M9.9 9.9a3 3 0 0 0 4.2 4.2"/>',
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14"/>',
  dice: '<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8.5" cy="8.5" r="1" fill="currentColor"/><circle cx="15.5" cy="15.5" r="1" fill="currentColor"/><circle cx="12" cy="12" r="1" fill="currentColor"/>',
  back: '<path d="m15 18-6-6 6-6"/>',
  archive: '<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8M10 12h4"/>',
  download: '<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>',
  upload: '<path d="M12 15V4M7 9l5-5 5 5M5 20h14"/>',
};

function icon(name) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('class', 'icon');
  svg.setAttribute('aria-hidden', 'true');
  svg.innerHTML = ICONS[name];
  return svg;
}

function hydrateIcons(root) {
  $$('i[data-icon]', root).forEach((el) => el.replaceWith(icon(el.dataset.icon)));
}

function setIcon(btn, name) {
  $('svg', btn).replaceWith(icon(name));
}

// API 呼叫
class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

async function api(method, path, body, { raw = false } = {}) {
  const opt = { method, headers: { 'X-Keychain': '1' }, credentials: 'same-origin' };
  if (body !== undefined) {
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  const res = await fetch(path, opt);
  if (res.status === 401 && path !== '/api/unlock') {
    showLock(true, '保險庫已上鎖，請重新解鎖');
    throw new ApiError('保險庫已上鎖', 401);
  }
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch { /* 忽略 */ }
    throw new ApiError(msg, res.status);
  }
  if (raw) return res;
  return res.status === 204 ? null : res.json();
}

// 狀態
const state = { entries: [], selected: null, mode: 'view', query: '', tag: '', dirty: false };
let pollTimer = null;

const el = {
  lock: $('#lock'), lockForm: $('#lock-form'), lockHint: $('#lock-hint'),
  lockPw: $('#lock-pw'), lockPw2: $('#lock-pw2'), lockErr: $('#lock-err'), lockSubmit: $('#lock-submit'),
  app: $('#app'), search: $('#search'), tags: $('#tags'), count: $('#count'),
  list: $('#list'), empty: $('#empty'), pane: $('#pane'), toast: $('#toast'),
  pwDialog: $('#pw-dialog'), pwForm: $('#pw-form'), pwErr: $('#pw-err'),
  bkDialog: $('#bk-dialog'), bkExport: $('#bk-export'), bkErr: $('#bk-err'),
  bkRestore: $('#bk-restore'), rsErr: $('#rs-err'), rsResult: $('#rs-result'),
};

// 上鎖畫面
function showLock(initialized, message = '') {
  clearInterval(pollTimer);
  Object.assign(state, { entries: [], selected: null, mode: 'view', query: '', tag: '', dirty: false });
  el.list.replaceChildren();
  el.pane.replaceChildren();
  el.search.value = '';
  $$('dialog[open]').forEach((d) => d.close());
  el.app.hidden = true;
  el.lock.hidden = false;

  const setup = !initialized;
  el.lockForm.dataset.mode = setup ? 'setup' : 'unlock';
  el.lockHint.textContent = setup ? '第一次使用，請設定主密碼（至少 8 個字元）。主密碼遺失將無法復原資料。' : '輸入主密碼以解鎖保險庫';
  el.lockPw.autocomplete = setup ? 'new-password' : 'current-password';
  el.lockPw2.hidden = !setup;
  el.lockSubmit.textContent = setup ? '建立保險庫' : '解鎖';
  el.lockPw.value = el.lockPw2.value = '';
  el.lockErr.textContent = message;
  el.lockPw.focus();
}

el.lockForm.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  const setup = el.lockForm.dataset.mode === 'setup';
  const pw = el.lockPw.value;
  if (!pw) return showLockError('請輸入主密碼');
  if (setup) {
    if ([...pw].length < 8) return showLockError('主密碼至少需 8 個字元');
    if (pw !== el.lockPw2.value) return showLockError('兩次輸入的主密碼不一致');
  }
  el.lockSubmit.disabled = true;
  const label = el.lockSubmit.textContent;
  el.lockSubmit.textContent = '處理中…';
  try {
    await api('POST', setup ? '/api/setup' : '/api/unlock', { password: pw });
    el.lockPw.value = el.lockPw2.value = '';
    await enterApp();
  } catch (err) {
    showLockError(err.message);
  } finally {
    el.lockSubmit.disabled = false;
    el.lockSubmit.textContent = label;
  }
});

function showLockError(msg) {
  el.lockErr.textContent = msg;
  el.lockPw.select();
}

// 主畫面
async function enterApp() {
  el.lock.hidden = true;
  el.app.hidden = false;
  el.app.classList.remove('show-pane');
  await loadEntries();
  renderPane();
  clearInterval(pollTimer);
  pollTimer = setInterval(checkStatus, 30_000);
}

async function checkStatus() {
  try {
    const st = await api('GET', '/api/status');
    if (!st.unlocked) showLock(st.initialized, '閒置過久，已自動上鎖');
  } catch { /* 忽略 */ }
}

async function loadEntries() {
  state.entries = await api('GET', '/api/entries');
  if (state.tag && !allTags().includes(state.tag)) state.tag = '';
  renderTags();
  renderList();
}

function allTags() {
  return [...new Set(state.entries.flatMap((e) => e.tags))].sort((a, b) => a.localeCompare(b, 'zh-Hant'));
}

function filtered() {
  const q = state.query.trim().toLowerCase();
  return state.entries.filter((e) => {
    if (state.tag && !e.tags.includes(state.tag)) return false;
    if (!q) return true;
    return [e.title, e.username, e.url, e.notes, ...e.tags].some((f) => f.toLowerCase().includes(q));
  });
}

function current() {
  return state.entries.find((e) => e.id === state.selected) || null;
}

function renderTags() {
  const tags = allTags();
  el.tags.hidden = tags.length === 0;
  const chips = ['', ...tags].map((t) => {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'chip';
    b.textContent = t || '全部';
    b.setAttribute('aria-pressed', String(state.tag === t));
    b.addEventListener('click', () => {
      state.tag = t;
      renderTags();
      renderList();
    });
    return b;
  });
  el.tags.replaceChildren(...chips);
}

function renderList() {
  const items = filtered();
  el.count.textContent = state.entries.length ? `${items.length} / ${state.entries.length} 筆` : '';
  el.list.replaceChildren(...items.map((e) => {
    const li = document.createElement('li');
    const b = document.createElement('button');
    b.type = 'button';
    b.setAttribute('aria-current', String(e.id === state.selected));
    const text = document.createElement('span');
    text.className = 'text';
    const title = document.createElement('span');
    title.className = 'title';
    title.textContent = e.title;
    const sub = document.createElement('span');
    sub.className = 'sub';
    sub.textContent = e.username || hostOf(e.url) || '—';
    text.append(title, sub);
    b.append(avatar(e.title), text);
    b.addEventListener('click', () => select(e.id));
    li.append(b);
    return li;
  }));
  el.empty.hidden = items.length > 0;
  el.empty.textContent = state.entries.length ? '沒有符合的結果' : '還沒有任何帳號，點「新增」開始建立吧';
}

function avatar(title, cls = '') {
  const a = document.createElement('div');
  a.className = `avatar ${cls}`.trim();
  a.textContent = ([...title.trim()][0] || '?').toUpperCase();
  let h = 0;
  for (const ch of title) h = (h * 31 + ch.codePointAt(0)) % 360;
  a.style.setProperty('--hue', h);
  return a;
}

function hostOf(url) {
  try { return new URL(url).host; } catch { return ''; }
}

function select(id) {
  if (!confirmDiscard()) return;
  state.selected = id;
  state.mode = 'view';
  renderList();
  renderPane();
  el.app.classList.add('show-pane');
}

function confirmDiscard() {
  if (state.mode !== 'view' && state.dirty && !confirm('尚未儲存的變更將會遺失，確定離開？')) return false;
  state.dirty = false;
  return true;
}

function goBack() {
  if (!confirmDiscard()) return;
  state.mode = 'view';
  el.app.classList.remove('show-pane');
  renderPane();
}

function renderPane() {
  const e = current();
  if (state.mode === 'new') return mount(buildForm(null));
  if (!e) {
    state.mode = 'view';
    return mount(clone('#tpl-placeholder'));
  }
  mount(state.mode === 'edit' ? buildForm(e) : buildView(e));
}

function mount(node) {
  el.pane.replaceChildren(node);
  el.pane.scrollTop = 0;
}

function clone(sel) {
  const node = $(sel).content.firstElementChild.cloneNode(true);
  hydrateIcons(node);
  return node;
}

// 檢視模式
function buildView(e) {
  const v = clone('#tpl-view');
  const f = (name) => $(`[data-f="${name}"]`, v);
  const row = (name, show) => { $(`[data-row="${name}"]`, v).hidden = !show; };

  f('avatar').replaceWith(avatar(e.title, 'lg'));
  f('title').textContent = e.title;
  f('meta').textContent = `更新於 ${fmtDate(e.updatedAt)}`;
  f('meta').title = `建立於 ${fmtDate(e.createdAt)}`;

  row('username', e.username);
  f('username').textContent = e.username;

  row('password', e.password);
  const pw = f('password');
  const mask = () => { pw.textContent = '•'.repeat(Math.min(e.password.length, 16)); };
  mask();
  const reveal = $('[data-act="reveal"]', v);
  reveal.addEventListener('click', () => {
    const shown = reveal.dataset.shown === '1';
    reveal.dataset.shown = shown ? '0' : '1';
    if (shown) mask(); else pw.textContent = e.password;
    setIcon(reveal, shown ? 'eye' : 'eyeOff');
    reveal.title = shown ? '顯示密碼' : '隱藏密碼';
  });

  row('url', e.url);
  const urlBox = f('url');
  if (/^https?:\/\//i.test(e.url)) {
    const a = document.createElement('a');
    a.href = e.url;
    a.target = '_blank';
    a.rel = 'noopener noreferrer';
    a.textContent = e.url;
    urlBox.append(a);
  } else {
    urlBox.textContent = e.url;
  }

  row('tags', e.tags.length);
  f('tags').replaceChildren(...e.tags.map((t) => {
    const s = document.createElement('span');
    s.className = 'chip';
    s.textContent = t;
    return s;
  }));

  row('notes', e.notes);
  f('notes').textContent = e.notes;

  $$('[data-copy]', v).forEach((b) => {
    const key = b.dataset.copy;
    b.addEventListener('click', () => copy(e[key], { username: '帳號', password: '密碼', url: '網址' }[key]));
  });
  $('[data-act="edit"]', v).addEventListener('click', () => {
    state.mode = 'edit';
    renderPane();
  });
  $('[data-act="delete"]', v).addEventListener('click', () => removeEntry(e));
  $('[data-act="back"]', v).addEventListener('click', goBack);
  return v;
}

async function removeEntry(e) {
  if (!confirm(`確定要刪除「${e.title}」嗎？此動作無法復原。`)) return;
  try {
    await api('DELETE', `/api/entries/${encodeURIComponent(e.id)}`);
    state.selected = null;
    el.app.classList.remove('show-pane');
    await loadEntries();
    renderPane();
    toast('已刪除');
  } catch (err) {
    if (err.status !== 401) toast(err.message, true);
  }
}

// 編輯模式
function buildForm(e) {
  const form = clone('#tpl-form');
  const isNew = !e;
  const f = (name) => $(`[data-f="${name}"]`, form);
  const inp = form.elements;

  f('heading').textContent = isNew ? '新增帳號' : '編輯帳號';
  inp.title.value = e?.title ?? '';
  inp.username.value = e?.username ?? '';
  inp.password.value = e?.password ?? '';
  inp.url.value = e?.url ?? '';
  inp.tags.value = e?.tags.join(', ') ?? '';
  inp.notes.value = e?.notes ?? '';

  const meter = $('.strength', form);
  const updateStrength = () => {
    const s = strength(inp.password.value);
    meter.dataset.level = s.level;
    f('strength').textContent = s.label;
  };
  const regen = () => {
    inp.password.value = generate(Number(inp.len.value), inp.sym.checked);
    updateStrength();
  };
  if (isNew) regen();
  else updateStrength();

  form.addEventListener('input', (ev) => {
    if (ev.target.name === 'len' || ev.target.name === 'sym') return;
    state.dirty = true;
    if (ev.target === inp.password) updateStrength();
  });
  inp.len.addEventListener('input', () => { f('len').textContent = inp.len.value; });

  const toggle = $('[data-act="toggle"]', form);
  toggle.addEventListener('click', () => {
    const show = inp.password.type === 'password';
    inp.password.type = show ? 'text' : 'password';
    setIcon(toggle, show ? 'eyeOff' : 'eye');
    toggle.title = show ? '隱藏密碼' : '顯示密碼';
  });
  $('[data-act="gen"]', form).addEventListener('click', () => {
    regen();
    state.dirty = true;
    if (inp.password.type === 'password') toggle.click();
  });

  const cancel = () => {
    if (!confirmDiscard()) return;
    state.mode = 'view';
    if (isNew) el.app.classList.remove('show-pane');
    renderPane();
  };
  $('[data-act="cancel"]', form).addEventListener('click', cancel);
  $('[data-act="back"]', form).addEventListener('click', goBack);
  form.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape') cancel();
  });

  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const body = {
      title: inp.title.value.trim(),
      username: inp.username.value.trim(),
      password: inp.password.value,
      url: inp.url.value.trim(),
      notes: inp.notes.value,
      tags: inp.tags.value.split(/[,，]/).map((t) => t.trim()).filter(Boolean),
    };
    if (!body.title) {
      f('err').textContent = '請輸入標題';
      inp.title.focus();
      return;
    }
    const submit = $('button[type="submit"]', form);
    submit.disabled = true;
    try {
      const saved = isNew
        ? await api('POST', '/api/entries', body)
        : await api('PUT', `/api/entries/${encodeURIComponent(e.id)}`, body);
      state.dirty = false;
      state.selected = saved.id;
      state.mode = 'view';
      await loadEntries();
      renderPane();
      toast('已儲存');
    } catch (err) {
      if (err.status !== 401) f('err').textContent = err.message;
      submit.disabled = false;
    }
  });

  requestAnimationFrame(() => inp.title.focus());
  return form;
}

// 密碼工具
const CHARSETS = {
  lower: 'abcdefghijkmnopqrstuvwxyz',
  upper: 'ABCDEFGHJKLMNPQRSTUVWXYZ',
  digit: '23456789',
  symbol: '!@#$%^&*-_=+?',
};

function randInt(n) {
  const limit = Math.floor(0x100000000 / n) * n;
  const buf = new Uint32Array(1);
  do crypto.getRandomValues(buf); while (buf[0] >= limit);
  return buf[0] % n;
}

function generate(len, symbols) {
  const sets = [CHARSETS.lower, CHARSETS.upper, CHARSETS.digit];
  if (symbols) sets.push(CHARSETS.symbol);
  const all = sets.join('');
  const out = sets.map((s) => s[randInt(s.length)]);
  while (out.length < len) out.push(all[randInt(all.length)]);
  for (let i = out.length - 1; i > 0; i--) {
    const j = randInt(i + 1);
    [out[i], out[j]] = [out[j], out[i]];
  }
  return out.join('');
}

function strength(pw) {
  if (!pw) return { level: 0, label: '' };
  let pool = 0;
  if (/[a-z]/.test(pw)) pool += 26;
  if (/[A-Z]/.test(pw)) pool += 26;
  if (/\d/.test(pw)) pool += 10;
  if (/[^a-zA-Z0-9]/.test(pw)) pool += 33;
  const bits = [...pw].length * Math.log2(pool);
  const level = bits < 40 ? 1 : bits < 60 ? 2 : bits < 80 ? 3 : 4;
  return { level, label: ['', '弱', '普通', '強', '很強'][level] };
}

// 剪貼簿
let clipTimer = null;

async function copy(text, label) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    toast('複製失敗，瀏覽器不允許存取剪貼簿', true);
    return;
  }
  toast(`已複製${label}，30 秒後自動清除`);
  clearTimeout(clipTimer);
  clipTimer = setTimeout(() => navigator.clipboard.writeText('').catch(() => {}), 30_000);
}

// 提示訊息
let toastTimer = null;

function toast(msg, bad = false) {
  el.toast.textContent = msg;
  el.toast.classList.toggle('bad', bad);
  el.toast.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.toast.hidden = true; }, 2600);
}

function fmtDate(s) {
  return new Date(s).toLocaleString('zh-TW', { dateStyle: 'medium', timeStyle: 'short' });
}

// 頂部操作
el.search.addEventListener('input', () => {
  state.query = el.search.value;
  renderList();
});

$('#add-btn').addEventListener('click', () => {
  if (!confirmDiscard()) return;
  state.mode = 'new';
  state.selected = null;
  renderList();
  renderPane();
  el.app.classList.add('show-pane');
});

$('#logout-btn').addEventListener('click', async () => {
  if (!confirmDiscard()) return;
  try { await api('POST', '/api/lock'); } catch { /* 忽略 */ }
  showLock(true);
});

$('#pw-btn').addEventListener('click', () => {
  el.pwForm.reset();
  el.pwErr.textContent = '';
  el.pwDialog.showModal();
});

$('#pw-cancel').addEventListener('click', () => el.pwDialog.close());

el.pwForm.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  const old = $('#pw-old').value;
  const next = $('#pw-new').value;
  if ([...next].length < 8) { el.pwErr.textContent = '新主密碼至少需 8 個字元'; return; }
  if (next !== $('#pw-new2').value) { el.pwErr.textContent = '兩次輸入的新主密碼不一致'; return; }
  const submit = $('button[type="submit"]', el.pwForm);
  submit.disabled = true;
  try {
    await api('POST', '/api/password', { old, new: next });
    el.pwDialog.close();
    toast('主密碼已更新');
  } catch (err) {
    if (err.status !== 401) el.pwErr.textContent = err.message;
  } finally {
    submit.disabled = false;
    el.pwForm.reset();
  }
});

// 備份與還原
const MAX_BACKUP = 32 * 1024 * 1024;

function showTab(name) {
  for (const tab of $$('[role="tab"]', el.bkDialog)) {
    const on = tab.id === `tab-${name}`;
    tab.setAttribute('aria-selected', String(on));
    tab.tabIndex = on ? 0 : -1;
    $(`#${tab.getAttribute('aria-controls')}`).hidden = !on;
  }
}

$('#bk-btn').addEventListener('click', () => {
  if (!confirmDiscard()) return;
  if (state.mode !== 'view') {
    state.mode = 'view';
    renderPane();
  }
  el.bkExport.reset();
  el.bkRestore.reset();
  el.bkErr.textContent = el.rsErr.textContent = '';
  el.rsResult.hidden = true;
  showTab('export');
  el.bkDialog.showModal();
  $('#bk-pw').focus();
});

$('#tab-export').addEventListener('click', () => showTab('export'));
$('#tab-restore').addEventListener('click', () => showTab('restore'));
$$('[data-close]', el.bkDialog).forEach((b) => b.addEventListener('click', () => el.bkDialog.close()));

el.bkExport.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  const pw = $('#bk-pw').value;
  if ([...pw].length < 8) { el.bkErr.textContent = '備份密碼至少需 8 個字元'; return; }
  if (pw !== $('#bk-pw2').value) { el.bkErr.textContent = '兩次輸入的備份密碼不一致'; return; }
  el.bkErr.textContent = '';
  const submit = $('button[type="submit"]', el.bkExport);
  submit.disabled = true;
  try {
    const res = await api('POST', '/api/backup', { password: pw }, { raw: true });
    const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') || '')?.[1] || 'keychain-backup.kcbak';
    saveBlob(await res.blob(), name);
    el.bkExport.reset();
    el.bkDialog.close();
    toast(`已下載 ${name}，請上傳到雲端硬碟保存`);
  } catch (err) {
    if (err.status !== 401) el.bkErr.textContent = err.message;
  } finally {
    submit.disabled = false;
  }
});

function saveBlob(blob, name) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

el.bkRestore.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  el.rsErr.textContent = '';
  el.rsResult.hidden = true;
  const file = $('#rs-file').files[0];
  const password = $('#rs-pw').value;
  const mode = el.bkRestore.elements.mode.value;
  if (!file) { el.rsErr.textContent = '請選擇備份檔案'; return; }
  if (file.size > MAX_BACKUP) { el.rsErr.textContent = '檔案過大（上限 32 MB）'; return; }
  if (!password) { el.rsErr.textContent = '請輸入備份密碼'; return; }

  let backup;
  try {
    backup = JSON.parse(await file.text());
  } catch {
    el.rsErr.textContent = '不是有效的 keychain 備份檔';
    return;
  }
  if (mode === 'replace' && state.entries.length
    && !confirm(`覆蓋將清空目前的 ${state.entries.length} 筆資料並以備份取代，此動作無法復原。確定繼續？`)) return;

  const submit = $('button[type="submit"]', el.bkRestore);
  submit.disabled = true;
  try {
    const r = await api('POST', '/api/restore', { password, mode, backup });
    $('#rs-pw').value = '';
    const parts = [`新增 ${r.added}`, `更新 ${r.updated}`, `未變更 ${r.unchanged}`];
    if (mode === 'replace') parts.push(`移除 ${r.removed}`);
    el.rsResult.textContent = `還原完成：${parts.join('、')}，目前共 ${r.total} 筆。`;
    el.rsResult.hidden = false;
    await loadEntries();
    if (!current()) {
      state.selected = null;
      el.app.classList.remove('show-pane');
    }
    renderPane();
  } catch (err) {
    if (err.status !== 401) el.rsErr.textContent = err.message;
  } finally {
    submit.disabled = false;
  }
});

document.addEventListener('keydown', (ev) => {
  const typing = ev.target.closest('input, textarea');
  if (ev.key === '/' && !typing && !el.app.hidden && !$('dialog[open]')) {
    ev.preventDefault();
    el.search.focus();
  }
});

window.addEventListener('beforeunload', (ev) => {
  if (state.mode !== 'view' && state.dirty) ev.preventDefault();
});

// 啟動
hydrateIcons(document);
(async () => {
  try {
    const st = await api('GET', '/api/status');
    if (st.unlocked) await enterApp();
    else showLock(st.initialized);
  } catch {
    document.body.textContent = '無法連線到伺服器';
  }
})();
