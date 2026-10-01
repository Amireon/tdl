(() => {
const $ = (sel) => document.querySelector(sel);
const API = '';

let auth = { state: 'connecting' };

async function getTasks() {
  const r = await fetch(API + '/api/tasks');
  return r.ok ? r.json() : [];
}

async function getAuth() {
  const r = await fetch(API + '/api/auth/status');
  return r.ok ? r.json() : { state: 'error' };
}

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
  ));
}

function fmtBytes(b) {
  if (!b && b !== 0) return '-';
  const units = ['B','KB','MB','GB','TB'];
  let i = 0;
  while (b >= 1024 && i < units.length - 1) { b /= 1024; i++; }
  return b.toFixed(2) + ' ' + units[i];
}

const ICONS = {
  plane: '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>',
  play: '<svg viewBox="0 0 24 24" width="15" height="15" fill="currentColor"><path d="M8 5.14v13.72a1 1 0 0 0 1.54.84l10.3-6.86a1 1 0 0 0 0-1.68L9.54 4.3A1 1 0 0 0 8 5.14z"/></svg>',
  stop: '<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><rect x="6" y="6" width="12" height="12" rx="2.5"/></svg>',
  restart: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg>',
  trash: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/></svg>',
  inbox: '<svg viewBox="0 0 24 24" width="46" height="46" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>',
  external: '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>',
  folderSm: '<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>',
  kind: {
    video: '<svg viewBox="0 0 24 24" width="17" height="17" fill="currentColor"><path d="M8 5.14v13.72a1 1 0 0 0 1.54.84l10.3-6.86a1 1 0 0 0 0-1.68L9.54 4.3A1 1 0 0 0 8 5.14z"/></svg>',
    image: '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2.5"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>',
    audio: '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>',
    archive: '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="21 8 21 21 3 21 3 8"/><rect x="1" y="3" width="22" height="5"/><line x1="10" y1="12" x2="14" y2="12"/></svg>',
    doc: '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/></svg>',
    other: '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>',
    pending: '<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>',
  },
};

const KIND_EXTS = {
  video: ['mp4','mov','mkv','avi','webm','flv','wmv','m4v','mpg','mpeg','3gp','ts','mts'],
  image: ['jpg','jpeg','png','gif','webp','bmp','svg','heic','heif','tiff','ico'],
  audio: ['mp3','flac','wav','m4a','aac','ogg','opus','wma','amr'],
  archive: ['zip','rar','7z','tar','gz','bz2','xz','tgz'],
  doc: ['pdf','doc','docx','xls','xlsx','ppt','pptx','txt','md','epub','csv','rtf'],
};

function fileKind(name) {
  const dot = name.lastIndexOf('.');
  const ext = dot >= 0 ? name.slice(dot + 1).toLowerCase() : '';
  for (const [kind, exts] of Object.entries(KIND_EXTS)) {
    if (exts.includes(ext)) return kind;
  }
  return 'other';
}

function shortHome(p) {
  return p.replace(/^\/(?:Users|home)\/[^/]+\//, '~/');
}

function renderTasks(list) {
  const tbody = $('#task-tbody');
  const count = $('#task-count');
  tbody.innerHTML = '';
  count.textContent = list.length;
  const statusText = { pending: '等待中', downloading: '下载中', done: '完成', error: '错误', canceled: '已取消' };

  if (!list.length) {
    tbody.innerHTML = `<tr><td colspan="6"><div class="empty-state">${ICONS.inbox}<p>还没有下载任务，粘贴上方链接添加第一个吧</p></div></td></tr>`;
    return;
  }

  for (const t of list) {
    const tr = document.createElement('tr');
    const pct = t.total > 0 ? Math.round(t.progress / t.total * 100) : 0;
    let barMod = '';
    if (t.status === 'done') barMod = 'is-done';
    else if (t.status === 'error') barMod = 'is-error';
    else if (t.status === 'canceled') barMod = 'is-canceled';

    let actions = '';
    if (t.status === 'pending' || t.status === 'error' || t.status === 'canceled') {
      actions += `<button class="icon-btn accent" data-act="start" data-id="${t.id}" title="开始下载">${ICONS.play}</button>`;
    }
    if (t.status === 'downloading') {
      actions += `<button class="icon-btn" data-act="cancel" data-id="${t.id}" title="取消下载">${ICONS.stop}</button>`;
    }
    if (t.status === 'done' || t.status === 'error' || t.status === 'canceled') {
      actions += `<button class="icon-btn" data-act="restart" data-id="${t.id}" title="重新开始">${ICONS.restart}</button>`;
    }
    actions += `<button class="icon-btn danger" data-act="delete" data-id="${t.id}" title="删除任务">${ICONS.trash}</button>`;

    const urlText = t.url.replace(/^https?:\/\//, '');
    const urlShort = urlText.length > 34 ? urlText.slice(0, 34) + '…' : urlText;
    const effDir = t.effective_dir || t.dir || '';
    const errShort = (t.error || '').slice(0, 60) + ((t.error || '').length > 60 ? '…' : '');
    const sizeText = t.total > 0 ? `${fmtBytes(t.progress)} / ${fmtBytes(t.total)}` : '—';

    const kind = t.filename ? fileKind(t.filename) : 'pending';
    const nameText = t.filename
      ? t.filename
      : (t.status === 'downloading' ? '正在下载…' : '下载后生成文件');
    const dirText = effDir ? shortHome(effDir) : '—';
    tr.innerHTML = `
      <td><span class="status-pill st-${t.status}" title="${esc(t.error || '')}"><span class="dot"></span>${statusText[t.status] || t.status}</span></td>
      <td><a class="link-cell" href="${esc(t.url)}" target="_blank" rel="noopener noreferrer" title="${esc(t.url)}">${ICONS.plane}<span class="link-text">${esc(urlShort)}</span></a></td>
      <td>${t.author ? `<span class="author-chip">${esc(t.author)}</span>` : '<span class="muted">—</span>'}</td>
      <td>
        <div class="save-card${effDir ? '' : ' is-static'}" data-act="open-dir" data-id="${t.id}" title="${effDir ? '在文件管理器中打开：' + effDir : '暂无可打开的目录'}">
          <span class="save-thumb kind-${kind}">${ICONS.kind[kind]}</span>
          <span class="save-meta">
            <span class="save-name${t.filename ? '' : ' is-empty'}" title="${esc(t.filename || '')}">${esc(nameText)}</span>
            <span class="save-path">${ICONS.folderSm}<span title="${esc(effDir)}">${esc(dirText)}</span></span>
          </span>
          <span class="save-go">${ICONS.external}</span>
        </div>
      </td>
      <td class="progress">
        <div class="progress-bar"><div class="progress-bar-inner ${barMod}" style="width:${pct}%"></div></div>
        <div class="progress-meta"><span>${sizeText}</span><span class="progress-pct">${pct}%</span></div>
        ${t.error ? `<div class="progress-err" title="${esc(t.error)}">${esc(errShort)}</div>` : ''}
      </td>
      <td><div class="actions">${actions}</div></td>
    `;
    tbody.appendChild(tr);
  }
}

function renderAuth() {
  const bar = $('#auth-status');
  const btn = $('#btn-auth');
  let stateClass = 'is-error';
  let text = '未登录';
  if (auth.state === 'ready') {
    const name = auth.first_name || auth.username || '';
    stateClass = 'is-ready';
    text = '已登录' + (name ? ' · ' + name : '');
    btn.textContent = '登录';
    btn.style.display = 'none';
  } else if (auth.state === 'connecting') {
    stateClass = 'is-connecting';
    text = '连接中…';
    btn.style.display = 'none';
  } else if (auth.state === 'wait_phone' || auth.state === 'wait_code' || auth.state === 'wait_password') {
    stateClass = 'is-wait';
    text = '等待登录…';
    btn.textContent = '继续登录';
    btn.style.display = '';
  } else {
    text = '未登录' + (auth.error ? ' · ' + auth.error : '');
    btn.textContent = '登录';
    btn.style.display = '';
  }
  bar.className = 'auth-pill ' + stateClass;
  bar.innerHTML = '<span class="auth-dot"></span>' + esc(text);
}

async function loadSettingsPlaceholder() {
  const r = await fetch(API + '/api/settings');
  if (!r.ok) return;
  const s = await r.json();
  let dir = (s.default_dir || '').replace(/^\/Users\/[^/]+\//, '~/');
  if (dir.length > 22) dir = '…' + dir.slice(-20);
  $('#inp-dir').placeholder = '下载目录（默认：' + (dir || '') + '）';
  $('#inp-author').placeholder = '作者（默认：' + (s.default_author || '无') + '）';
}

async function poll() {
  try {
    const [tasks, a] = await Promise.all([getTasks(), getAuth()]);
    auth = a;
    renderAuth();
    if (loginModal.style.display === 'flex') {
      if (auth.state === 'ready') {
        loginModal.style.display = 'none';
      } else {
        updateLoginStep();
      }
    }
    renderTasks(tasks);
  } catch (e) { console.error(e); }
}

// Add task
$('#add-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const body = {
    url: $('#inp-url').value.trim(),
    author: $('#inp-author').value.trim(),
    dir: $('#inp-dir').value.trim(),
  };
  const r = await fetch(API + '/api/tasks', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (r.ok) {
    $('#inp-url').value = '';
    poll();
  } else {
    const t = await r.text();
    alert('添加失败: ' + t);
  }
});

// Task actions
document.addEventListener('click', async (e) => {
  const el = e.target.closest('[data-act]');
  if (el && el.classList.contains('is-static')) return;
  if (!el) return;
  const act = el.dataset.act;
  const id = el.dataset.id;
  if (act === 'delete') {
    if (!confirm('确认删除该任务？')) return;
    await fetch(API + '/api/tasks/' + id, { method: 'DELETE' });
  } else if (act === 'start') {
    await fetch(API + '/api/tasks/' + id + '/start', { method: 'POST' });
  } else if (act === 'cancel') {
    await fetch(API + '/api/tasks/' + id + '/cancel', { method: 'POST' });
  } else if (act === 'restart') {
    await fetch(API + '/api/tasks/' + id + '/restart', { method: 'POST' });
  } else if (act === 'open-dir') {
    const r = await fetch(API + '/api/tasks/' + id + '/open-dir', { method: 'POST' });
    if (!r.ok) alert('打开目录失败: ' + await r.text());
  }
  poll();
});

// Start all
$('#btn-start-all').addEventListener('click', async () => {
  await fetch(API + '/api/tasks/start-all', { method: 'POST' });
  poll();
});

// Clear finished
$('#btn-clear-done').addEventListener('click', async () => {
  if (!confirm('清空所有已完成任务？')) return;
  await fetch(API + '/api/tasks/clear-finished', { method: 'POST' });
  poll();
});

// Login modal
const loginModal = $('#login-modal');
const loginClose = $('#login-close');
$('#btn-auth').addEventListener('click', () => { loginModal.style.display = 'flex'; updateLoginStep(); });
loginClose.addEventListener('click', () => { loginModal.style.display = 'none'; });

function updateLoginStep() {
  const s = auth.state;
  $('#login-step-phone').style.display = (s === 'wait_phone' || s === 'error' || s === 'connecting') ? '' : 'none';
  $('#login-step-code').style.display = s === 'wait_code' ? '' : 'none';
  $('#login-step-password').style.display = s === 'wait_password' ? '' : 'none';
  $('#login-error').textContent = auth.error || '';
}

$('#btn-send-code').addEventListener('click', async () => {
  const phone = $('#login-phone').value.trim();
  if (!phone) return;
  const r = await fetch(API + '/api/auth/phone', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ phone }) });
  if (!r.ok) { const t = await r.text(); $('#login-error').textContent = t; }
  else { poll(); }
});
$('#btn-submit-code').addEventListener('click', async () => {
  const code = $('#login-code').value.trim();
  if (!code) return;
  const r = await fetch(API + '/api/auth/code', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ code }) });
  if (!r.ok) { const t = await r.text(); $('#login-error').textContent = t; }
  else { $('#login-code').value = ''; poll(); }
});
$('#btn-submit-password').addEventListener('click', async () => {
  const password = $('#login-password').value.trim();
  if (!password) return;
  const r = await fetch(API + '/api/auth/password', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password }) });
  if (!r.ok) { const t = await r.text(); $('#login-error').textContent = t; }
  else { $('#login-password').value = ''; poll(); }
});

// Settings modal
const settingsModal = $('#settings-modal');
$('#btn-settings').addEventListener('click', async () => {
  const r = await fetch(API + '/api/settings');
  if (!r.ok) return;
  const s = await r.json();
  $('#set-proxy').value = s.proxy || '';
  $('#set-default-dir').value = s.default_dir || '';
  $('#set-default-author').value = s.default_author || '';
  $('#set-max-concurrent').value = s.max_concurrent_tasks || 1;
  $('#set-threads').value = s.threads || 4;
  $('#set-limit').value = s.limit || 2;
  $('#set-rewrite-ext').checked = s.rewrite_ext;
  $('#set-skip-same').checked = s.skip_same;
  settingsModal.style.display = 'flex';
});
$('#settings-close').addEventListener('click', () => { settingsModal.style.display = 'none'; });
$('#settings-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const body = {
    proxy: $('#set-proxy').value.trim(),
    default_dir: $('#set-default-dir').value.trim(),
    default_author: $('#set-default-author').value.trim(),
    max_concurrent_tasks: parseInt($('#set-max-concurrent').value) || 1,
    threads: parseInt($('#set-threads').value) || 4,
    limit: parseInt($('#set-limit').value) || 2,
    rewrite_ext: $('#set-rewrite-ext').checked,
    skip_same: $('#set-skip-same').checked,
  };
  const r = await fetch(API + '/api/settings', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (r.ok) { settingsModal.style.display = 'none'; poll(); loadSettingsPlaceholder(); }
  else { alert('保存失败: ' + await r.text()); }
});

window.addEventListener('click', (e) => {
  if (e.target === loginModal) loginModal.style.display = 'none';
  if (e.target === settingsModal) settingsModal.style.display = 'none';
});

loadSettingsPlaceholder();
poll();
setInterval(poll, 1000);
})();
