/* ============================================
   ScrapeOwl Dashboard — app.js
   Full client-side application
   ============================================ */

'use strict';

// ==================== State ====================
const state = {
  ws: null,
  jobs: [],
  runs: [],
  stats: { total_runs: 0, total_records: 0, total_errors: 0, active_runs: 0 },
  currentPage: 'dashboard',
  activityLog: [],
  wsReconnectTimer: null,
  wsReconnectDelay: 1000,
};

const MAX_ACTIVITY = 200;

// ==================== Init ====================
document.addEventListener('DOMContentLoaded', () => {
  initParticles();
  connectWebSocket();
  initEditor();
  showPage('dashboard');
  refreshAll();
});

// ==================== Navigation ====================
function showPage(page) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));

  const pageEl = document.getElementById(`page-${page}`);
  const navEl = document.getElementById(`nav-${page}`);

  if (pageEl) {
    pageEl.classList.add('active');
    state.currentPage = page;
  }
  if (navEl) navEl.classList.add('active');

  // Load data for page
  switch (page) {
    case 'dashboard':
      loadStats();
      loadRecentRuns();
      loadDashboardJobs();
      break;
    case 'jobs':
      loadJobs();
      break;
    case 'runs':
      loadRuns();
      break;
    case 'new-job':
      updateLineNumbers();
      break;
  }
}

// ==================== WebSocket ====================
function connectWebSocket() {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${location.host}/ws`;

  state.ws = new WebSocket(wsUrl);

  state.ws.onopen = () => {
    setWSStatus('connected');
    state.wsReconnectDelay = 1000;
    if (state.wsReconnectTimer) {
      clearTimeout(state.wsReconnectTimer);
      state.wsReconnectTimer = null;
    }
  };

  state.ws.onmessage = (evt) => {
    try {
      const event = JSON.parse(evt.data);
      handleWSEvent(event);
    } catch (e) {
      console.error('WS parse error:', e);
    }
  };

  state.ws.onclose = () => {
    setWSStatus('disconnected');
    scheduleReconnect();
  };

  state.ws.onerror = () => {
    setWSStatus('disconnected');
  };
}

function scheduleReconnect() {
  if (state.wsReconnectTimer) return;
  state.wsReconnectTimer = setTimeout(() => {
    state.wsReconnectTimer = null;
    setWSStatus('connecting');
    connectWebSocket();
    state.wsReconnectDelay = Math.min(state.wsReconnectDelay * 2, 30000);
  }, state.wsReconnectDelay);
}

function setWSStatus(status) {
  const dot = document.querySelector('.status-dot');
  const text = document.querySelector('.status-text');
  if (!dot || !text) return;

  dot.className = `status-dot ${status}`;
  const labels = { connected: 'Connected', disconnected: 'Disconnected', connecting: 'Connecting...' };
  text.textContent = labels[status] || status;
}

function handleWSEvent(event) {
  if (!event.type) return;

  // Add to activity feed
  if (event.type === 'log' || event.type === 'step' || event.type === 'status' || event.type === 'extract') {
    addActivity(event);
  }

  // Handle specific event types
  switch (event.type) {
    case 'status':
      if (event.data && event.data.status === 'success' || event.data && event.data.status === 'failed') {
        // Refresh relevant data
        setTimeout(() => {
          if (state.currentPage === 'dashboard') {
            loadStats();
            loadRecentRuns();
          } else if (state.currentPage === 'runs') {
            loadRuns();
          }
        }, 500);
      }
      break;
    case 'complete':
      showToast('success', 'Job Complete', `${event.job_name} finished successfully`);
      break;
    case 'error':
      showToast('error', 'Job Failed', event.message || `${event.job_name} encountered an error`);
      break;
  }
}

// ==================== Activity Feed ====================
function addActivity(event) {
  state.activityLog.unshift(event);
  if (state.activityLog.length > MAX_ACTIVITY) {
    state.activityLog = state.activityLog.slice(0, MAX_ACTIVITY);
  }
  renderActivityFeed();
}

function renderActivityFeed() {
  const feed = document.getElementById('activity-feed');
  if (!feed) return;

  if (state.activityLog.length === 0) {
    feed.innerHTML = `
      <div class="activity-empty">
        <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm.75-11.25a.75.75 0 00-1.5 0v4.59L7.3 9.24a.75.75 0 00-1.1 1.02l3.25 3.5a.75.75 0 001.1 0l3.25-3.5a.75.75 0 10-1.1-1.02l-1.95 2.1V6.75z"/></svg>
        <p>Waiting for job activity...</p>
      </div>`;
    return;
  }

  const items = state.activityLog.slice(0, 50).map(event => {
    const time = new Date(event.timestamp).toLocaleTimeString('en-US', { hour12: false });
    const level = event.level || 'info';
    let icon = '›';
    let msg = event.message || '';

    if (event.type === 'step') {
      icon = event.data?.success ? '✓' : '✗';
      msg = `${event.data?.action} ${event.data?.selector || ''}`;
    } else if (event.type === 'extract') {
      icon = '↳';
      msg = `Extracted: ${event.data?.name}`;
    } else if (event.type === 'status') {
      icon = '●';
      msg = `Status: ${event.data?.status} (${event.data?.progress || 0}%)`;
    }

    return `
      <div class="activity-item level-${level}" role="listitem">
        <span class="activity-time">${escHtml(time)}</span>
        <span class="activity-icon">${icon}</span>
        <span class="activity-msg">${escHtml(msg)}</span>
        ${event.job_name ? `<span class="activity-job">${escHtml(event.job_name)}</span>` : ''}
      </div>`;
  }).join('');

  feed.innerHTML = items;
}

// ==================== API Calls ====================
async function apiFetch(path, options = {}) {
  try {
    const res = await fetch(path, {
      headers: { 'Content-Type': 'application/json', ...options.headers },
      ...options,
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
    return data;
  } catch (err) {
    throw err;
  }
}

// ==================== Stats ====================
async function loadStats() {
  try {
    const data = await apiFetch('/api/stats');
    state.stats = data;
    updateStatsUI(data);
  } catch (e) {
    console.error('loadStats:', e);
  }
}

function updateStatsUI(data) {
  const setVal = (id, val) => {
    const el = document.getElementById(id);
    if (el) {
      el.textContent = formatNumber(val);
      el.classList.add('stat-animate');
      setTimeout(() => el.classList.remove('stat-animate'), 600);
    }
  };
  setVal('total-runs', data.total_runs || 0);
  setVal('total-records', data.total_records || 0);
  setVal('active-runs', data.active_runs || 0);

  const rate = data.total_runs > 0
    ? Math.round(((data.total_runs - (data.total_errors || 0)) / data.total_runs) * 100)
    : 100;
  const rateEl = document.getElementById('success-rate');
  if (rateEl) rateEl.textContent = `${rate}%`;
}

// ==================== Jobs ====================
async function loadJobs() {
  try {
    const jobs = await apiFetch('/api/jobs');
    state.jobs = jobs || [];
    renderJobs(state.jobs);
  } catch (e) {
    console.error('loadJobs:', e);
    showToast('error', 'Error', 'Failed to load jobs');
  }
}

async function loadDashboardJobs() {
  try {
    const jobs = await apiFetch('/api/jobs');
    state.jobs = jobs || [];
    renderDashboardJobs(state.jobs);
  } catch (e) {
    console.error('loadDashboardJobs:', e);
  }
}

function renderJobs(jobs) {
  const el = document.getElementById('jobs-grid');
  if (!el) return;

  if (!jobs || jobs.length === 0) {
    el.innerHTML = `
      <div class="empty-state" style="grid-column:1/-1">
        <div class="empty-icon">🦉</div>
        <div class="empty-title">No jobs yet</div>
        <div class="empty-desc">Create your first scraping job to get started</div>
        <button class="btn btn-primary" onclick="showPage('new-job')" id="create-first-job">Create Job</button>
      </div>`;
    return;
  }

  el.innerHTML = jobs.map(job => renderJobCard(job)).join('');
}

function renderJobCard(job) {
  const yaml = job.yaml_content || '';
  // Parse start_url from YAML
  const urlMatch = yaml.match(/start_url:\s*["']?([^"'\n]+)["']?/);
  const startUrl = urlMatch ? urlMatch[1].trim() : '';
  const stepsMatch = yaml.match(/steps:/g);
  const extractorsMatch = yaml.match(/extractors:/g);
  const hasSchedule = !!job.schedule;

  return `
    <div class="job-card" onclick="showJobDetail('${escAttr(job.id)}')" id="job-card-${escAttr(job.id)}" role="article" aria-label="Job: ${escAttr(job.name)}">
      <div class="job-card-header">
        <div>
          <div class="job-card-name">${escHtml(job.name)}</div>
          ${startUrl ? `<div class="job-card-url" title="${escAttr(startUrl)}">${escHtml(truncateUrl(startUrl))}</div>` : ''}
        </div>
        <div class="job-card-enabled">
          <span class="status-badge ${job.enabled ? 'status-success' : 'status-pending'}">
            ${job.enabled ? 'Active' : 'Disabled'}
          </span>
        </div>
      </div>
      <div class="job-card-meta">
        ${hasSchedule ? `<span class="job-meta-tag">⏰ ${escHtml(job.schedule)}</span>` : ''}
        ${job.next_run ? `<span class="job-meta-tag">Next: ${formatDateTime(job.next_run)}</span>` : ''}
        <span class="job-meta-tag">📅 ${formatDate(job.created_at)}</span>
      </div>
      <div class="job-card-actions" onclick="event.stopPropagation()">
        <button class="btn btn-run btn-sm" onclick="runJobNow('${escAttr(job.id)}', '${escAttr(job.name)}')" id="run-job-${escAttr(job.id)}">
          ▶ Run Now
        </button>
        <button class="btn btn-ghost btn-sm" onclick="showJobDetail('${escAttr(job.id)}')" id="view-job-${escAttr(job.id)}">
          View
        </button>
        <button class="btn btn-danger btn-sm" onclick="deleteJob('${escAttr(job.id)}', '${escAttr(job.name)}')" id="delete-job-${escAttr(job.id)}">
          Delete
        </button>
      </div>
    </div>`;
}

function renderDashboardJobs(jobs) {
  const el = document.getElementById('dashboard-jobs-list');
  if (!el) return;

  if (!jobs || jobs.length === 0) {
    el.innerHTML = `<div class="empty-state"><div class="empty-desc">No jobs created yet</div></div>`;
    return;
  }

  el.innerHTML = `
    <table class="runs-table" role="grid">
      <thead>
        <tr>
          <th scope="col">Name</th>
          <th scope="col">Schedule</th>
          <th scope="col">Next Run</th>
          <th scope="col">Status</th>
          <th scope="col">Actions</th>
        </tr>
      </thead>
      <tbody>
        ${jobs.map(job => `
          <tr>
            <td style="font-weight:500;color:var(--text-primary)">${escHtml(job.name)}</td>
            <td style="font-family:var(--font-mono);font-size:0.8rem">${escHtml(job.schedule || '—')}</td>
            <td>${job.next_run ? formatDateTime(job.next_run) : '—'}</td>
            <td><span class="status-badge ${job.enabled ? 'status-success' : 'status-pending'}">${job.enabled ? 'Active' : 'Disabled'}</span></td>
            <td>
              <div style="display:flex;gap:0.4rem">
                <button class="btn btn-run btn-sm" onclick="runJobNow('${escAttr(job.id)}', '${escAttr(job.name)}')" id="dash-run-${escAttr(job.id)}">▶ Run</button>
                <button class="btn btn-ghost btn-sm" onclick="showJobDetail('${escAttr(job.id)}')" id="dash-view-${escAttr(job.id)}">View</button>
              </div>
            </td>
          </tr>
        `).join('')}
      </tbody>
    </table>`;
}

async function runJobNow(id, name) {
  try {
    const btn = document.getElementById(`run-job-${id}`) || document.getElementById(`dash-run-${id}`);
    if (btn) { btn.disabled = true; btn.textContent = '⏳ Starting...'; }

    const result = await apiFetch(`/api/jobs/${id}/run`, { method: 'POST' });
    showToast('success', 'Job Started', `Run ID: ${result.run_id.slice(0, 8)}...`);

    setTimeout(() => {
      if (btn) { btn.disabled = false; btn.textContent = '▶ Run Now'; }
      if (state.currentPage === 'dashboard') {
        loadStats();
        loadRecentRuns();
      } else if (state.currentPage === 'runs') {
        loadRuns();
      }
    }, 2000);
  } catch (e) {
    showToast('error', 'Failed to Start', e.message);
    const btn = document.getElementById(`run-job-${id}`) || document.getElementById(`dash-run-${id}`);
    if (btn) { btn.disabled = false; btn.textContent = '▶ Run Now'; }
  }
}

async function deleteJob(id, name) {
  if (!confirm(`Delete job "${name}"?\n\nThis will also delete all run history for this job.`)) return;
  try {
    await apiFetch(`/api/jobs/${id}`, { method: 'DELETE' });
    showToast('success', 'Deleted', `Job "${name}" deleted`);
    loadJobs();
    loadDashboardJobs();
  } catch (e) {
    showToast('error', 'Delete Failed', e.message);
  }
}

async function createJob() {
  const yaml = document.getElementById('yaml-editor')?.value?.trim();
  if (!yaml) {
    showToast('warning', 'Empty Editor', 'Please enter a YAML job definition');
    return;
  }

  const btn = document.getElementById('create-job-submit-btn');
  if (btn) { btn.disabled = true; btn.textContent = 'Creating...'; }

  try {
    const job = await apiFetch('/api/jobs', {
      method: 'POST',
      body: JSON.stringify({ yaml_content: yaml }),
    });
    showToast('success', 'Job Created', `"${job.name}" is ready to run`);
    document.getElementById('yaml-editor').value = '';
    updateLineNumbers();
    updateEditorStatus('Job created successfully!');
    showPage('jobs');
  } catch (e) {
    showToast('error', 'Creation Failed', e.message);
    updateEditorStatus(`Error: ${e.message}`);
  } finally {
    if (btn) { btn.disabled = false; btn.innerHTML = '<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 3a1 1 0 011 1v5h5a1 1 0 110 2h-5v5a1 1 0 11-2 0v-5H4a1 1 0 110-2h5V4a1 1 0 011-1z" clip-rule="evenodd"/></svg> Create Job'; }
  }
}

// ==================== Runs ====================
async function loadRecentRuns() {
  try {
    const runs = await apiFetch('/api/runs');
    state.runs = runs || [];
    renderRecentRuns(state.runs.slice(0, 8));
  } catch (e) {
    console.error('loadRecentRuns:', e);
  }
}

async function loadRuns() {
  try {
    const status = document.getElementById('runs-filter-status')?.value;
    let url = '/api/runs';
    const runs = await apiFetch(url);
    state.runs = runs || [];
    renderRunsTable(state.runs);
  } catch (e) {
    console.error('loadRuns:', e);
    showToast('error', 'Error', 'Failed to load runs');
  }
}

function filterRuns() {
  const status = document.getElementById('runs-filter-status')?.value;
  let filtered = state.runs;
  if (status) filtered = state.runs.filter(r => r.status === status);
  renderRunsTable(filtered);
}

function renderRecentRuns(runs) {
  const el = document.getElementById('recent-runs-list');
  if (!el) return;

  if (!runs || runs.length === 0) {
    el.innerHTML = `<div class="empty-state"><div class="empty-desc">No runs yet. Start a job to see activity here.</div></div>`;
    return;
  }

  el.innerHTML = runs.map(run => `
    <div class="run-item" onclick="showRunModal('${escAttr(run.id)}')" role="button" tabindex="0" id="run-item-${escAttr(run.id)}">
      <div class="run-job-name">${escHtml(run.job_name)}</div>
      <div class="run-meta">
        <span class="run-records">${run.records || 0} rec</span>
        <span class="run-time">${formatTimeAgo(run.created_at)}</span>
        <span class="status-badge status-${run.status}">${run.status}</span>
      </div>
    </div>`).join('');
}

function renderRunsTable(runs) {
  const el = document.getElementById('runs-table-body');
  if (!el) return;

  if (!runs || runs.length === 0) {
    el.innerHTML = `<tr><td colspan="6" class="table-loading"><div class="empty-desc">No runs found</div></td></tr>`;
    return;
  }

  el.innerHTML = runs.map(run => {
    const duration = run.started_at && run.completed_at
      ? formatDuration(new Date(run.started_at), new Date(run.completed_at))
      : run.started_at ? 'Running...' : '—';

    return `
      <tr>
        <td>${escHtml(run.job_name)}</td>
        <td><span class="status-badge status-${run.status}">${run.status}</span></td>
        <td>${run.started_at ? formatDateTime(run.started_at) : '—'}</td>
        <td style="font-family:var(--font-mono);font-size:0.8rem">${duration}</td>
        <td style="font-family:var(--font-mono)">${run.records || 0}</td>
        <td>
          <div style="display:flex;gap:0.4rem">
            <button class="btn btn-ghost btn-sm" onclick="showRunModal('${escAttr(run.id)}')" id="view-run-${escAttr(run.id)}">View Log</button>
            ${run.status === 'running' ? `<button class="btn btn-danger btn-sm" onclick="stopRun('${escAttr(run.id)}')" id="stop-run-${escAttr(run.id)}">Stop</button>` : ''}
          </div>
        </td>
      </tr>`;
  }).join('');
}

async function stopRun(id) {
  try {
    await apiFetch(`/api/runs/${id}/stop`, { method: 'POST' });
    showToast('info', 'Stopping', 'Run is being stopped...');
    setTimeout(loadRuns, 1500);
  } catch (e) {
    showToast('error', 'Failed to Stop', e.message);
  }
}

// ==================== Job Detail ====================
async function showJobDetail(id) {
  try {
    const [job, runs] = await Promise.all([
      apiFetch(`/api/jobs/${id}`),
      apiFetch(`/api/runs?job_id=${id}`),
    ]);

    const actionsEl = document.getElementById('job-detail-actions');
    if (actionsEl) {
      actionsEl.innerHTML = `
        <button class="btn btn-run" onclick="runJobNow('${escAttr(job.id)}', '${escAttr(job.name)}')" id="detail-run-${escAttr(job.id)}">▶ Run Now</button>
        <button class="btn btn-danger" onclick="deleteJob('${escAttr(job.id)}', '${escAttr(job.name)}')" id="detail-delete-${escAttr(job.id)}">Delete</button>`;
    }

    const content = document.getElementById('job-detail-content');
    if (content) {
      content.innerHTML = `
        <div class="job-detail-header">
          <h2 class="job-detail-name">${escHtml(job.name)}</h2>
          <span class="status-badge ${job.enabled ? 'status-success' : 'status-pending'}">${job.enabled ? 'Active' : 'Disabled'}</span>
        </div>
        <div class="job-detail-grid">
          <div class="card">
            <div class="card-header"><h3 class="card-title">YAML Definition</h3></div>
            <pre class="yaml-preview">${escHtml(job.yaml_content || '')}</pre>
          </div>
          <div class="card">
            <div class="card-header">
              <h3 class="card-title">Job Info</h3>
            </div>
            <div style="padding:1.25rem;display:flex;flex-direction:column;gap:0.875rem">
              <div><div style="font-size:0.75rem;color:var(--text-muted);margin-bottom:0.25rem">JOB ID</div><div style="font-family:var(--font-mono);font-size:0.8rem">${escHtml(job.id)}</div></div>
              ${job.schedule ? `<div><div style="font-size:0.75rem;color:var(--text-muted);margin-bottom:0.25rem">SCHEDULE</div><div style="font-family:var(--font-mono);font-size:0.8rem;color:var(--accent-primary)">${escHtml(job.schedule)}</div></div>` : ''}
              <div><div style="font-size:0.75rem;color:var(--text-muted);margin-bottom:0.25rem">CREATED</div><div>${formatDateTime(job.created_at)}</div></div>
              <div><div style="font-size:0.75rem;color:var(--text-muted);margin-bottom:0.25rem">UPDATED</div><div>${formatDateTime(job.updated_at)}</div></div>
            </div>
          </div>
        </div>
        <div class="card">
          <div class="card-header"><h3 class="card-title">Run History</h3></div>
          <table class="runs-table">
            <thead>
              <tr>
                <th scope="col">Run ID</th>
                <th scope="col">Status</th>
                <th scope="col">Started</th>
                <th scope="col">Duration</th>
                <th scope="col">Records</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              ${!runs || runs.length === 0
                ? '<tr><td colspan="6" class="table-loading"><div class="empty-desc">No runs yet</div></td></tr>'
                : runs.map(run => `
                  <tr>
                    <td style="font-family:var(--font-mono);font-size:0.8rem">${run.id.slice(0, 8)}...</td>
                    <td><span class="status-badge status-${run.status}">${run.status}</span></td>
                    <td>${run.started_at ? formatDateTime(run.started_at) : '—'}</td>
                    <td style="font-family:var(--font-mono);font-size:0.8rem">${run.started_at && run.completed_at ? formatDuration(new Date(run.started_at), new Date(run.completed_at)) : '—'}</td>
                    <td style="font-family:var(--font-mono)">${run.records || 0}</td>
                    <td><button class="btn btn-ghost btn-sm" onclick="showRunModal('${escAttr(run.id)}')" id="detail-run-view-${escAttr(run.id)}">View</button></td>
                  </tr>`).join('')
              }
            </tbody>
          </table>
        </div>`;
    }

    showPage('job-detail');
  } catch (e) {
    showToast('error', 'Error', `Failed to load job: ${e.message}`);
  }
}

// ==================== Run Modal ====================
async function showRunModal(runId) {
  const modal = document.getElementById('run-modal');
  const title = document.getElementById('modal-title');
  const logEl = document.getElementById('run-log');
  if (!modal || !logEl) return;

  title.textContent = `Run: ${runId.slice(0, 8)}...`;
  logEl.innerHTML = '<div class="loading-state"><div class="spinner"></div><span>Loading run details...</span></div>';
  modal.hidden = false;

  try {
    const run = await apiFetch(`/api/runs/${runId}`);
    logEl.innerHTML = `
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin-bottom:1rem;padding:0 0.5rem">
        <div><div style="font-size:0.7rem;color:var(--text-muted);margin-bottom:0.25rem">STATUS</div><span class="status-badge status-${run.status}">${run.status}</span></div>
        <div><div style="font-size:0.7rem;color:var(--text-muted);margin-bottom:0.25rem">RECORDS</div><div style="font-family:var(--font-mono)">${run.records || 0}</div></div>
        <div><div style="font-size:0.7rem;color:var(--text-muted);margin-bottom:0.25rem">STARTED</div><div>${run.started_at ? formatDateTime(run.started_at) : '—'}</div></div>
        <div><div style="font-size:0.7rem;color:var(--text-muted);margin-bottom:0.25rem">COMPLETED</div><div>${run.completed_at ? formatDateTime(run.completed_at) : '—'}</div></div>
      </div>
      ${run.error ? `<div style="background:var(--danger-bg);border:1px solid rgba(239,68,68,0.2);border-radius:8px;padding:0.875rem;margin-bottom:1rem;color:var(--danger);font-family:var(--font-mono);font-size:0.8rem">${escHtml(run.error)}</div>` : ''}
      <div style="color:var(--text-muted);font-size:0.8rem;padding:0 0.5rem">
        <p>Detailed logs are streamed live via WebSocket during execution.</p>
        <p style="margin-top:0.5rem">Run ID: <span style="font-family:var(--font-mono);color:var(--accent-primary)">${escHtml(run.id)}</span></p>
      </div>`;
  } catch (e) {
    logEl.innerHTML = `<div style="color:var(--danger);padding:1rem">Failed to load run: ${escHtml(e.message)}</div>`;
  }
}

function closeModal() {
  const modal = document.getElementById('run-modal');
  if (modal) modal.hidden = true;
}

// Close modal on Escape
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') closeModal();
});

// ==================== YAML Editor ====================
function initEditor() {
  const editor = document.getElementById('yaml-editor');
  if (!editor) return;

  editor.addEventListener('input', () => {
    updateLineNumbers();
    updateEditorStatus('Ready');
    document.getElementById('validate-result')?.classList.add('hidden');
  });

  editor.addEventListener('scroll', syncScroll);
  editor.addEventListener('keydown', handleEditorKeydown);
}

function handleEditorKeydown(e) {
  if (e.key === 'Tab') {
    e.preventDefault();
    const start = e.target.selectionStart;
    const end = e.target.selectionEnd;
    const spaces = '  ';
    e.target.value = e.target.value.substring(0, start) + spaces + e.target.value.substring(end);
    e.target.selectionStart = e.target.selectionEnd = start + spaces.length;
    updateLineNumbers();
  }
}

function updateLineNumbers() {
  const editor = document.getElementById('yaml-editor');
  const lineNums = document.getElementById('line-numbers');
  if (!editor || !lineNums) return;

  const lines = (editor.value.split('\n').length) || 1;
  lineNums.textContent = Array.from({ length: lines }, (_, i) => i + 1).join('\n');
}

function syncScroll() {
  const editor = document.getElementById('yaml-editor');
  const lineNums = document.getElementById('line-numbers');
  if (lineNums) lineNums.scrollTop = editor.scrollTop;
}

function updateEditorStatus(msg) {
  const el = document.getElementById('editor-status');
  if (el) el.textContent = msg;
}

function clearEditor() {
  const editor = document.getElementById('yaml-editor');
  if (editor) {
    editor.value = '';
    updateLineNumbers();
    updateEditorStatus('Ready');
  }
  document.getElementById('validate-result')?.classList.add('hidden');
}

async function validateYAML() {
  const yaml = document.getElementById('yaml-editor')?.value?.trim();
  if (!yaml) {
    showToast('warning', 'Empty Editor', 'Please enter a YAML job definition');
    return;
  }

  const badge = document.getElementById('validate-result');
  try {
    const result = await apiFetch('/api/validate', {
      method: 'POST',
      body: JSON.stringify({ yaml_content: yaml }),
    });

    if (badge) {
      badge.classList.remove('hidden');
      if (result.valid) {
        badge.textContent = '✓ Valid';
        badge.className = 'validate-badge valid';
        updateEditorStatus('YAML is valid');
      } else {
        badge.textContent = '✗ Invalid';
        badge.className = 'validate-badge invalid';
        updateEditorStatus(`Error: ${result.error}`);
      }
    }
  } catch (e) {
    showToast('error', 'Validation Failed', e.message);
  }
}

// ==================== Templates ====================
const YAML_TEMPLATES = {
  basic: `name: "my-scraper"
start_url: "https://example.com"
steps:
  - action: wait
    selector: "body"
    wait: 1s
extractors:
  - name: title
    type: css
    selector: "h1"
    attribute: text
  - name: description
    type: css
    selector: "meta[name=description]"
    attribute: content
output:
  format: jsonl
  path: "./output/data.jsonl"
`,
  full: `name: "product-scraper"
start_url: "https://example.com/products"
steps:
  - action: click
    selector: "button.load-more"
    wait: 2s
    optional: true
  - action: type
    selector: "input#search"
    text: "laptop"
    wait: 1s
  - action: click
    selector: "button.search-btn"
    wait: 3s
extractors:
  - name: title
    type: css
    selector: "h1.product-title"
    attribute: text
  - name: price
    type: css
    selector: ".price .value"
    attribute: text
  - name: description
    type: ai
    prompt: "Extract the full product description text. Return a JSON object with key 'description'."
  - name: image_url
    type: css
    selector: "img.product-image"
    attribute: src
output:
  format: jsonl
  path: "./output/products.jsonl"
proxy:
  type: static
  list:
    - "http://user:pass@proxy1:8080"
    - "http://user:pass@proxy2:8080"
captcha:
  provider: 2captcha
  api_key: "\${CAPTCHA_API_KEY}"
ai:
  provider: openai
  api_key: "\${OPENAI_API_KEY}"
  model: "gpt-4o"
retry:
  max_attempts: 3
  backoff: "exponential"
schedule: "0 */6 * * *"
`,
};

function loadTemplate(name) {
  const editor = document.getElementById('yaml-editor');
  if (!editor) return;
  editor.value = YAML_TEMPLATES[name] || '';
  updateLineNumbers();
  updateEditorStatus('Template loaded');
  document.getElementById('validate-result')?.classList.add('hidden');
}

// ==================== Refresh ====================
async function refreshAll() {
  const btn = document.getElementById('refresh-btn');
  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Refreshing...';
  }

  await Promise.allSettled([loadStats(), loadRecentRuns(), loadDashboardJobs()]);

  if (btn) {
    btn.disabled = false;
    btn.innerHTML = `<svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M4 2a1 1 0 011 1v2.101a7.002 7.002 0 0111.601 2.566 1 1 0 11-1.885.666A5.002 5.002 0 005.999 7H9a1 1 0 010 2H4a1 1 0 01-1-1V3a1 1 0 011-1zm.008 9.057a1 1 0 011.276.61A5.002 5.002 0 0014.001 13H11a1 1 0 110-2h5a1 1 0 011 1v5a1 1 0 11-2 0v-2.101a7.002 7.002 0 01-11.601-2.566 1 1 0 01.61-1.276z" clip-rule="evenodd"/></svg> Refresh`;
  }
}

// ==================== Toast Notifications ====================
function showToast(type, title, message, duration = 4000) {
  const container = document.getElementById('toast-container');
  if (!container) return;

  const icons = { success: '✓', error: '✗', warning: '⚠', info: 'ℹ' };
  const id = `toast-${Date.now()}`;

  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.id = id;
  toast.innerHTML = `
    <span class="toast-icon">${icons[type] || 'ℹ'}</span>
    <div class="toast-body">
      <div class="toast-title">${escHtml(title)}</div>
      ${message ? `<div class="toast-message">${escHtml(message)}</div>` : ''}
    </div>`;

  container.appendChild(toast);
  setTimeout(() => {
    toast.style.animation = 'none';
    toast.style.opacity = '0';
    toast.style.transform = 'translateX(24px)';
    toast.style.transition = 'all 0.3s ease';
    setTimeout(() => toast.remove(), 300);
  }, duration);
}

// ==================== Particles ====================
function initParticles() {
  const canvas = document.getElementById('particles');
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  let W = canvas.width = window.innerWidth;
  let H = canvas.height = window.innerHeight;

  const particles = Array.from({ length: 60 }, () => ({
    x: Math.random() * W,
    y: Math.random() * H,
    r: Math.random() * 1.5 + 0.5,
    vx: (Math.random() - 0.5) * 0.3,
    vy: (Math.random() - 0.5) * 0.3,
    alpha: Math.random() * 0.4 + 0.1,
  }));

  function draw() {
    ctx.clearRect(0, 0, W, H);

    // Draw particles
    particles.forEach(p => {
      ctx.beginPath();
      ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
      ctx.fillStyle = `rgba(0, 255, 198, ${p.alpha})`;
      ctx.fill();

      p.x += p.vx;
      p.y += p.vy;
      if (p.x < 0) p.x = W;
      if (p.x > W) p.x = 0;
      if (p.y < 0) p.y = H;
      if (p.y > H) p.y = 0;
    });

    // Draw connection lines
    particles.forEach((a, i) => {
      particles.slice(i + 1).forEach(b => {
        const dx = a.x - b.x;
        const dy = a.y - b.y;
        const dist = Math.sqrt(dx * dx + dy * dy);
        if (dist < 120) {
          ctx.beginPath();
          ctx.moveTo(a.x, a.y);
          ctx.lineTo(b.x, b.y);
          ctx.strokeStyle = `rgba(0, 255, 198, ${0.06 * (1 - dist / 120)})`;
          ctx.lineWidth = 0.5;
          ctx.stroke();
        }
      });
    });

    requestAnimationFrame(draw);
  }

  draw();

  window.addEventListener('resize', () => {
    W = canvas.width = window.innerWidth;
    H = canvas.height = window.innerHeight;
  });
}

// ==================== Utilities ====================
function escHtml(str) {
  if (str == null) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function escAttr(str) {
  if (str == null) return '';
  return String(str).replace(/["'<>&]/g, c => ({
    '"': '&quot;', "'": '&#39;', '<': '&lt;', '>': '&gt;', '&': '&amp;',
  }[c]));
}

function formatNumber(n) {
  if (n === undefined || n === null) return '0';
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
  return String(n);
}

function formatDate(dateStr) {
  if (!dateStr) return '—';
  try {
    return new Date(dateStr).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  } catch { return '—'; }
}

function formatDateTime(dateStr) {
  if (!dateStr) return '—';
  try {
    return new Date(dateStr).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  } catch { return '—'; }
}

function formatTimeAgo(dateStr) {
  if (!dateStr) return '—';
  try {
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'Just now';
    if (mins < 60) return `${mins}m ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
  } catch { return '—'; }
}

function formatDuration(start, end) {
  if (!start || !end) return '—';
  const ms = end - start;
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
}

function truncateUrl(url, max = 40) {
  if (!url) return '';
  try {
    const u = new URL(url.startsWith('http') ? url : 'https://' + url);
    const display = u.hostname + u.pathname;
    return display.length > max ? display.slice(0, max - 3) + '...' : display;
  } catch {
    return url.length > max ? url.slice(0, max - 3) + '...' : url;
  }
}
