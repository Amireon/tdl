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

function renderTasks(list) {
  const tbody = $('#task-tbody');
  tbody.innerHTML = '';
  const statusText = { pending: '等待中', downloading: '下载中', done: '完成', error: '错误', canceled: '已取消' };
  for (const t of list) {
    const tr = document.createElement('tr');
    const pct = t.total > 0 ? Math.round(t.progress / t.total * 100) : 0;
    const badgeClass = 'badge-' + t.status;
    let actions = '';
    if (t.status === 'pending' || t.status === 'error' || t.status === 'canceled') {
      actions += `<button class="primary" data-act="start" data-id="${t.id}">开始</button>`;
    }
    if (t.status === 'downloading') {
      actions += `<button data-act="cancel" data-id="${t.id}">取消</button>`;
    }
    if (t.status === 'done' || t.status === 'error' || t.status === 'canceled') {
      actions += `<button data-act="restart" data-id="${t.id}">重新开始</button>`;
    }
    actions += `<button class="danger" data-act="delete" data-id="${t.id}">删除</button>`;

    const urlShort = t.url.slice(0, 40) + (t.url.length > 40 ? '…' : '');
    const effDir = t.effective_dir || t.dir || '';
    const errShort = (t.error || '').slice(0, 60) + ((t.error || '').length > 60 ? '…' : '');
    tr.innerHTML = `
      <td><span class="badge ${badgeClass}" title="${esc(t.error)}">${statusText[t.status] || t.status}</span></td>
      <td title="${esc(t.url)}"><a href="${esc(t.url)}" target="_blank" rel="noopener noreferrer">${esc(urlShort)}</a></td>
      <td>${esc(t.author || '-')}</td>
      <td>
        <div class="file-cell">
          <div class="file-line">
            <span class="file-name${t.filename ? '' : ' is-empty'}" title="${esc(t.filename || '')}">${esc(t.filename || '文件名待生成')}</span>
            <button class="dir-open" data-act="open-dir" data-id="${t.id}" title="在文件管理器中打开此目录" ${effDir ? '' : 'disabled'}>打开</button>
          </div>
          <div class="file-path" title="${esc(effDir)}">${esc(effDir || '-')}</div>
        </div>
      </td>
      <td class="progress">
        <div class="progress-bar"><div class="progress-bar-inner" style="width:${pct}%"></div></div>
        <div class="progress-text">${fmtBytes(t.progress)} / ${fmtBytes(t.total)} (${pct}%)</div>
        ${t.error ? `<div class="progress-text" style="color:#ff4d4f" title="${esc(t.error)}">${esc(errShort)}</div>` : ''}
      </td>
      <td><div class="actions">${actions}</div></td>
    `;
    tbody.appendChild(tr);
  }
}

function renderAuth() {
  const bar = $('#auth-status');
  const btn = $('#btn-auth');
  if (auth.state === 'ready') {
    const name = auth.first_name || auth.username || '';
    bar.textContent = '已登录' + (name ? ' (' + name + ')' : '');
    btn.textContent = '登录';
    btn.style.display = 'none';
  } else if (auth.state === 'connecting') {
    bar.textContent = '连接中…';
    btn.style.display = 'none';
  } else if (auth.state === 'wait_phone' || auth.state === 'wait_code' || auth.state === 'wait_password') {
    bar.textContent = '等待登录…';
    btn.textContent = '继续登录';
    btn.style.display = '';
  } else {
    bar.textContent = '未登录' + (auth.error ? ' (' + auth.error + ')' : '');
    btn.textContent = '登录';
    btn.style.display = '';
  }
}

async function loadSettingsPlaceholder() {
  const r = await fetch(API + '/api/settings');
  if (!r.ok) return;
  const s = await r.json();
  $('#inp-dir').placeholder = '下载目录 (默认: ' + (s.default_dir || '') + ')';
  $('#inp-author').placeholder = '作者 (默认: ' + (s.default_author || '无') + ')';
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
  const el = e.target.closest('button[data-act]');
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
