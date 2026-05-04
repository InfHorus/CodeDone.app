'use strict';

// ── State ─────────────────────────────────────────────────────────────────

const DEFAULT_CONFIG = {
    // Agents
    provider: 'deepseek', model: 'deepseek-v4-flash', cmModel: 'deepseek-v4-pro', implementerModel: 'deepseek-v4-flash', apiKey: '', keys: {},
    cmCount: 1, maxAgents: 4, agentTimeout: 1200, maxTokens: 8192,
    enableTemperature: false, temperature: 0.3, enableFinalizer: true,
    // Git
    autoCreateBranch: true, requireCleanTree: false, autoCommit: false,
    branchPrefix: 'codedone/work-', gitPath: '', lastWorkDir: '',
    // Console
    showTimestamps: true, autoScroll: true,
    // Appearance
    theme: 'dark',
};

const state = {
    session: null,
    config: { ...DEFAULT_CONFIG },
    workDir: '~/',
    hasSpawned: false,
    sessionMode: 'plan',
    tickets: [],
};

// ── Elements ──────────────────────────────────────────────────────────────

const $ = id => document.getElementById(id);

const el = {
    titlebar:        document.querySelector('.titlebar'),
    messagesList:    $('messagesList'),
    emptyState:      $('emptyState'),
    taskInput:       $('taskInput'),
    sendBtn:         $('sendBtn'),
    modeSwitch:      $('modeSwitch'),
    modePlan:        $('modePlan'),
    modeBuild:       $('modeBuild'),
    emptyTitle:      $('emptyTitle'),
    emptySub:        $('emptySub'),
    statusDot:       $('statusDot'),
    statusLabel:     $('statusLabel'),
    branchLabel:     $('branchLabel'),
    providerLabel:   $('providerLabel'),
    modelLabel:      $('modelLabel'),
    workdirLabel:    $('workdirLabel'),
    pathLabel:       $('pathLabel'),
    inputWorkdir:    $('inputWorkdir'),
    settingsBtn:     $('settingsBtn'),
    settingsOverlay: $('settingsOverlay'),
    settingsClose:   $('settingsClose'),
    settingsSave:    $('settingsSave'),
    providerSelect:  $('providerSelect'),
    cmModelInput:    $('cmModelInput'),
    implementerModelInput: $('implementerModelInput'),
    apiKeyInput:     $('apiKeyInput'),
    minimizeBtn:     $('minimizeBtn'),
    closeBtn:        $('closeBtn'),
    newSessionBtn:   $('newSessionBtn'),
    questionsOverlay: $('questionsOverlay'),
    questionsBody:    $('questionsBody'),
    questionsSubmit:  $('questionsSubmit'),
    questionsHeaderSub: $('questionsHeaderSub'),
    backlogBtn:       $('backlogBtn'),
    backlogBadge:     $('backlogBadge'),
    backlogOverlay:   $('backlogOverlay'),
    backlogClose:     $('backlogClose'),
    backlogBody:      $('backlogBody'),
    backlogHeaderCount: $('backlogHeaderCount'),
    backlogEmpty:     $('backlogEmpty'),
    activityStrip:    $('activityStrip'),
    activityActor:    $('activityActor'),
    activityDetailText: $('activityDetailText'),
    activityTools:    $('activityTools'),
    activityClockText: $('activityClockText'),
};

// ── Provider display names ────────────────────────────────────────────────

const PROVIDER_LABELS = {
    deepseek:   'DeepSeek',
    anthropic:  'Anthropic',
    openai:     'OpenAI',
    openrouter: 'OpenRouter',
};

const PROVIDER_DEFAULT_MODELS = {
    deepseek: {
        cmModel: 'deepseek-v4-pro',
        implementerModel: 'deepseek-v4-flash',
    },
    openrouter: {
        cmModel: 'anthropic/claude-sonnet-4.6',
        implementerModel: 'qwen/qwen3.6-27b',
    },
    openai: {
        cmModel: 'gpt-5.4',
        implementerModel: 'gpt-5.4-mini',
    },
    anthropic: {
        cmModel: 'claude-opus-4-7',
        implementerModel: 'claude-sonnet-4-6',
    },
};

function providerLabel(p) {
    return PROVIDER_LABELS[p] || p;
}

function setBodyState(name, enabled) {
    document.body.classList.toggle(name, !!enabled);
}

function setWindowMoving(enabled) {
    setBodyState('window-moving', enabled);
}

const SESSION_MODES = {
    plan: {
        label: 'Plan',
        statusIdle: 'Plan idle',
        emptyTitle: 'Ready to plan',
        emptySub: 'Ask the Contre-Maitre to inspect the repo, reason through options, and answer without dispatching work.',
        placeholder: 'Ask the Contre-Maitre to inspect and plan...',
        startTitle: 'Start planning session (Enter)',
        runningPlaceholder: 'Planning session running - input locked',
    },
    build: {
        label: 'Build',
        statusIdle: 'Build idle',
        emptyTitle: 'Ready to build',
        emptySub: 'Describe a task and CodeDone will coordinate agents to implement it feature by feature.',
        placeholder: 'Describe a task for the agents...',
        startTitle: 'Start build session (Enter)',
        runningPlaceholder: 'Build session running - input locked',
    },
};

function applySessionMode(mode) {
    if (!SESSION_MODES[mode]) mode = 'plan';
    state.sessionMode = mode;
    const cfg = SESSION_MODES[mode];

    document.body.dataset.sessionMode = mode;
    [el.modePlan, el.modeBuild].forEach(btn => {
        if (!btn) return;
        const active = btn.dataset.mode === mode;
        btn.classList.toggle('active', active);
        btn.setAttribute('aria-selected', active ? 'true' : 'false');
    });
    if (el.emptyTitle) el.emptyTitle.textContent = cfg.emptyTitle;
    if (el.emptySub) el.emptySub.textContent = cfg.emptySub;
    if (el.taskInput && !el.taskInput.hasAttribute('readonly')) {
        el.taskInput.placeholder = cfg.placeholder;
    }
    if (!state.session || ['idle', 'done', 'error'].includes(state.session.status)) {
        setStatus('idle', cfg.statusIdle);
    }
    applyInputLock(el.sendBtn?.dataset.mode === 'pause' ? 'running' : el.sendBtn?.dataset.mode === 'resume' ? 'paused' : 'idle');
}

// ── Wails bridge (graceful fallback for browser dev) ─────────────────────

const go = {
    async startSession(task, workDir, mode) {
        if (typeof window.go?.main?.App?.StartSession === 'function') {
            return window.go.main.App.StartSession(task, workDir, mode || 'build');
        }
        throw new Error('CodeDone app bridge is unavailable');
    },
    async cancelSession() {
        if (typeof window.go?.main?.App?.CancelSession === 'function') {
            return window.go.main.App.CancelSession();
        }
    },
    async pauseSession() {
        if (typeof window.go?.main?.App?.PauseSession === 'function') {
            return window.go.main.App.PauseSession();
        }
    },
    async resumeSession() {
        if (typeof window.go?.main?.App?.ResumeSession === 'function') {
            return window.go.main.App.ResumeSession();
        }
    },
    async resetSession() {
        if (typeof window.go?.main?.App?.ResetSession === 'function') {
            return window.go.main.App.ResetSession();
        }
    },
    async getConfig() {
        if (typeof window.go?.main?.App?.GetConfig === 'function') {
            const raw = await window.go.main.App.GetConfig();
            // Merge over defaults so Go zero-values don't overwrite JS defaults
            return { ...DEFAULT_CONFIG, ...raw };
        }
        return { ...DEFAULT_CONFIG, ...state.config };
    },
    async saveConfig(cfg) {
        if (typeof window.go?.main?.App?.SaveConfig === 'function') {
            return window.go.main.App.SaveConfig(cfg);
        }
        state.config = cfg;
    },
    async getPendingQuestions() {
        if (typeof window.go?.main?.App?.GetPendingQuestions === 'function') {
            return window.go.main.App.GetPendingQuestions();
        }
        return null;
    },
    async submitQuestionAnswers(answers) {
        if (typeof window.go?.main?.App?.SubmitQuestionAnswers === 'function') {
            return window.go.main.App.SubmitQuestionAnswers(answers);
        }
    },
    async getWorkDir() {
        if (typeof window.go?.main?.App?.GetWorkDir === 'function') {
            return window.go.main.App.GetWorkDir();
        }
        return '';
    },
    async getCurrentBranch(workDir) {
        if (typeof window.go?.main?.App?.GetCurrentBranch === 'function') {
            return window.go.main.App.GetCurrentBranch(workDir || '');
        }
        return '';
    },
};

// ── Wails events ──────────────────────────────────────────────────────────

function setupWailsEvents() {
    if (typeof window.runtime?.EventsOn !== 'function') return;
    window.runtime.EventsOn('message', msg => {
        appendMessage(msg);
        scrollToBottom();
    });
    window.runtime.EventsOn('session_status', payload => {
        if (!payload || !payload.status) return;
        setStatus(payload.status, payload.label || payload.status);
        applySessionClock(payload);
    });
    window.runtime.EventsOn('questions_pending', payload => {
        handlePendingQuestions(payload);
    });
    window.runtime.EventsOn('activity', payload => {
        applyActivity(payload);
    });
    window.runtime.EventsOn('backlog_update', tickets => {
        state.tickets = tickets || [];
        renderBacklog(state.tickets);
    });
}

// ── Live activity strip ───────────────────────────────────────────────────

const activityState = {
    startedAt: 0,    // unix ms when the session started running (0 = no active run)
    frozenMs: 0,     // last elapsed ms recorded when the session left "running"
    ticking: false,
    lastDetail: '',
    tools: [],
};

function applySessionClock(payload) {
    const status = payload.status;
    if (status === 'running' && payload.startedAt) {
        activityState.startedAt = payload.startedAt;
        activityState.frozenMs = 0;
        showActivityStrip();
        startActivityTicker();
        // Reset detail to a neutral state — will be overwritten by the next
        // activity event the moment the model emits one.
        if (!activityState.lastDetail) {
            setActivityDetail('Spinning up agents');
        }
    } else if (status === 'waiting') {
        // Session paused on user input — freeze the clock but keep the strip
        // visible so the user can see how long the run has been going.
        if (activityState.startedAt) {
            activityState.frozenMs = Date.now() - activityState.startedAt;
        }
        stopActivityTicker();
        setActivityDetail('Awaiting your input');
        if (el.activityStrip) el.activityStrip.dataset.kind = 'waiting';
    } else if (status === 'paused') {
        // User-initiated pause — same freeze behavior, different label.
        if (activityState.startedAt) {
            activityState.frozenMs = Date.now() - activityState.startedAt;
        }
        stopActivityTicker();
        setActivityDetail('Paused — click resume to continue');
        if (el.activityStrip) el.activityStrip.dataset.kind = 'paused';
    } else if (status === 'done' || status === 'error') {
        if (activityState.startedAt) {
            activityState.frozenMs = Date.now() - activityState.startedAt;
        }
        stopActivityTicker();
        // Don't collapse — morph the strip into a final summary card showing
        // "Session complete" + total time. The strip stays on screen until the
        // user starts a new session or hits the reset button.
        if (status === 'done' && state.sessionMode === 'plan') {
            settlePlanActivityStrip();
            return;
        }
        morphActivityStripToSummary(status, activityState.frozenMs);
    } else if (status === 'idle') {
        // Idle = explicit cancel/reset. Tear it down immediately.
        if (activityState.startedAt) {
            activityState.frozenMs = Date.now() - activityState.startedAt;
        }
        stopActivityTicker();
        clearTimeout(activityState._hideTimer);
        hideActivityStrip();
        activityState.startedAt = 0;
        activityState.frozenMs = 0;
        activityState.lastDetail = '';
        activityState.tools = [];
        renderActivityTools();
    }
}

function settlePlanActivityStrip() {
    const strip = el.activityStrip;
    if (!strip) return;
    clearTimeout(activityState._hideTimer);
    strip.classList.remove('is-summary', 'is-summary-error', 'is-morphing-in', 'is-morphing-out');
    strip.dataset.kind = 'complete';
    strip.dataset.role = 'contre-maitre';
    setActivityActor('Contre-Maitre');
    setActivityDetail('Response ready');
    activityState._hideTimer = setTimeout(() => {
        hideActivityStrip();
        activityState.startedAt = 0;
        activityState.frozenMs = 0;
        activityState.lastDetail = '';
        activityState.tools = [];
        renderActivityTools();
    }, 1100);
}

// morphActivityStripToSummary plays a swap animation: the live strip dissolves,
// then a fresh summary card slides in showing the terminal status and the
// total elapsed time. Implemented by toggling state classes on the same DOM
// element so layout doesn't jump.
function morphActivityStripToSummary(status, totalMs) {
    const strip = el.activityStrip;
    if (!strip) return;
    if (strip.classList.contains('is-summary') || strip.classList.contains('is-morphing-out')) return;
    clearTimeout(activityState._hideTimer);

    const isError = status === 'error';
    const label   = isError ? 'Session failed' : 'Session complete';
    const elapsed = formatElapsed(totalMs);

    // Phase 1 — dissolve current contents.
    strip.classList.add('is-morphing-out');

    setTimeout(() => {
        // Hold opacity at 0 before removing is-morphing-out so there's no
        // flash when the morph-out forwards-fill is released and the base
        // activitySlideIn end-state (opacity:1) would briefly show through.
        const inner = strip.querySelector('.activity-strip-inner');
        if (inner) inner.style.opacity = '0';

        strip.classList.remove('is-morphing-out');
        strip.classList.add('is-summary');
        if (isError) strip.classList.add('is-summary-error');
        else strip.classList.remove('is-summary-error');
        strip.dataset.kind = isError ? 'failed' : 'complete';
        delete strip.dataset.role;

        // Repaint the strip's inner block as a summary card. Keep the same
        // DOM ids so subsequent renders/resets remain consistent.
        strip.querySelector('.activity-strip-inner').innerHTML = `
            <span class="activity-summary-icon ${isError ? 'is-error' : ''}">
                ${isError
                    ? '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="8" r="6"/><line x1="8" y1="5" x2="8" y2="9"/><line x1="8" y1="11.5" x2="8" y2="11.51"/></svg>'
                    : '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="2.5 8.5 6.5 12.5 13.5 4.5"/></svg>'
                }
            </span>
            <span class="activity-summary-label">${label}</span>
            <span class="activity-divider">·</span>
            <span class="activity-summary-time">
                <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="8" cy="8" r="6"/>
                    <path d="M8 5v3l2 1"/>
                </svg>
                <span>took ${elapsed}</span>
            </span>
            <span class="activity-spacer"></span>
            <button class="activity-summary-dismiss" id="activitySummaryDismiss" title="Dismiss">
                <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round">
                    <line x1="4" y1="4" x2="12" y2="12"/>
                    <line x1="12" y1="4" x2="4" y2="12"/>
                </svg>
            </button>
        `;

        // Phase 2 — slide the new card in. Clear the inline opacity lock first
        // so the activityMorphIn animation (which also starts at opacity:0) owns
        // the transition without fighting an inline override.
        requestAnimationFrame(() => {
            const freshInner = strip.querySelector('.activity-strip-inner');
            if (freshInner) freshInner.style.opacity = '';
            strip.classList.add('is-morphing-in');
            setTimeout(() => {
                strip.classList.remove('is-morphing-in');
                // Freeze the inner so removing is-morphing-in doesn't cause the
                // browser to restart activitySlideIn (animation-name change = replay).
                const inner = strip.querySelector('.activity-strip-inner');
                if (inner) inner.style.animation = 'none';
            }, 360);
        });

        const dismiss = document.getElementById('activitySummaryDismiss');
        if (dismiss) dismiss.addEventListener('click', () => {
            hideActivityStrip();
            activityState.startedAt = 0;
            activityState.frozenMs = 0;
            activityState.lastDetail = '';
        });
    }, 220);
}

function showActivityStrip() {
    if (!el.activityStrip) return;
    el.activityStrip.classList.remove('is-hidden');
    el.activityStrip.setAttribute('aria-hidden', 'false');
}

function hideActivityStrip() {
    if (!el.activityStrip) return;
    el.activityStrip.classList.add('is-hidden');
    el.activityStrip.classList.remove('is-summary', 'is-summary-error', 'is-morphing-in', 'is-morphing-out');
    el.activityStrip.setAttribute('aria-hidden', 'true');
    el.activityStrip.removeAttribute('data-role');
    el.activityStrip.removeAttribute('data-kind');
    activityState.tools = [];
    // Restore the live-mode inner template so the next session reuses it cleanly.
    const inner = el.activityStrip.querySelector('.activity-strip-inner');
    if (inner) {
        inner.style.animation = '';
        inner.style.opacity = '';
        inner.innerHTML = `
            <div class="activity-pulse">
                <span class="activity-pulse-dot"></span>
                <span class="activity-pulse-ring"></span>
            </div>
            <div class="activity-actor" id="activityActor">Agents</div>
            <div class="activity-divider">·</div>
            <div class="activity-detail" id="activityDetail">
                <svg class="activity-detail-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="8" cy="8" r="6"/>
                    <path d="M8 5v3l2 1"/>
                </svg>
                <span class="activity-detail-text" id="activityDetailText">Spinning up</span>
            </div>
            <div class="activity-tools" id="activityTools" aria-live="polite"></div>
            <div class="activity-spacer"></div>
            <div class="activity-clock" id="activityClock" title="Elapsed since session start">
                <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                    <circle cx="8" cy="8" r="6"/>
                    <path d="M8 5v3l2 1"/>
                </svg>
                <span class="activity-clock-text" id="activityClockText">0:00</span>
            </div>
        `;
        // Re-bind the cached element refs to the freshly-rendered nodes.
        el.activityActor = document.getElementById('activityActor');
        el.activityDetailText = document.getElementById('activityDetailText');
        el.activityTools = document.getElementById('activityTools');
        el.activityClockText = document.getElementById('activityClockText');
    }
}

function startActivityTicker() {
    if (activityState.ticking) return;
    activityState.ticking = true;
    renderActivityClock();
    activityState._interval = setInterval(renderActivityClock, 1000);
}

function stopActivityTicker() {
    activityState.ticking = false;
    if (activityState._interval) {
        clearInterval(activityState._interval);
        activityState._interval = null;
    }
    renderActivityClock();
}

function renderActivityClock() {
    if (!el.activityClockText) return;
    let elapsedMs = 0;
    if (activityState.startedAt) {
        if (activityState.ticking) {
            elapsedMs = Date.now() - activityState.startedAt;
        } else if (activityState.frozenMs > 0) {
            elapsedMs = activityState.frozenMs;
        } else {
            elapsedMs = Date.now() - activityState.startedAt;
        }
    }
    el.activityClockText.textContent = formatElapsed(elapsedMs);
}

// formatElapsed renders `M:SS` until 60min, then `H:MM:SS`. Spec from product:
// minutes/seconds always; hours appear only once we're past 59:59.
function formatElapsed(ms) {
    if (!isFinite(ms) || ms < 0) ms = 0;
    const total = Math.floor(ms / 1000);
    const h = Math.floor(total / 3600);
    const m = Math.floor((total % 3600) / 60);
    const s = total % 60;
    const ss = String(s).padStart(2, '0');
    if (h <= 0) {
        return `${m}:${ss}`;
    }
    return `${h}:${String(m).padStart(2, '0')}:${ss}`;
}

function setActivityDetail(text) {
    if (!el.activityDetailText) return;
    if (activityState.lastDetail === text) return;
    activityState.lastDetail = text;
    // Crossfade so rapid tool changes feel smooth instead of jumpy.
    el.activityDetailText.classList.add('fading');
    setTimeout(() => {
        el.activityDetailText.textContent = text;
        el.activityDetailText.classList.remove('fading');
    }, 110);
}

function setActivityActor(label) {
    if (!el.activityActor) return;
    if (label && label.trim()) {
        el.activityActor.textContent = label;
    }
}

function rememberActivityTool(payload) {
    if (!payload.tool) return;
    const key = [
        payload.actor || '',
        payload.ticketId || '',
        payload.tool || '',
        payload.target || '',
        payload.detail || '',
    ].join('|');
    const existing = activityState.tools.find(item => item.key === key && item.status === 'running');
    const item = existing || {
        key,
        tool: payload.tool,
        target: payload.target || '',
        detail: payload.detail || '',
        actor: payload.actorLabel || payload.actor || '',
        ticketId: payload.ticketId || '',
        time: payload.time || Date.now(),
        status: 'running',
    };
    item.status = payload.status || (payload.kind === 'tool_failed' ? 'failed' : payload.kind === 'tool_done' ? 'done' : 'running');
    item.kind = payload.kind || '';
    if (!existing) activityState.tools.unshift(item);
    activityState.tools = activityState.tools.slice(0, 8);
    renderActivityTools();
}

function renderActivityTools() {
    if (!el.activityTools) return;
    const tools = activityState.tools.slice(0, 4);
    if (tools.length === 0) {
        el.activityTools.innerHTML = '';
        el.activityTools.classList.remove('has-tools');
        return;
    }
    const hidden = Math.max(0, activityState.tools.length - tools.length);
    el.activityTools.classList.add('has-tools');
    el.activityTools.innerHTML = tools.map(item => {
        const status = item.status || 'running';
        const title = item.detail || [item.tool, item.target].filter(Boolean).join(' · ');
        const target = item.target ? `<span class="activity-tool-target">${escHtml(shortenToolTarget(item.target))}</span>` : '';
        return `
            <div class="activity-tool-row ${escHtml(status)}" title="${escHtml(title)}">
                <span class="activity-tool-status"></span>
                <span class="activity-tool-name">${escHtml(item.tool)}</span>
                ${target}
            </div>
        `;
    }).join('') + (hidden ? `<div class="activity-tool-more">+${hidden} more</div>` : '');
}

function shortenToolTarget(target) {
    target = String(target || '').trim();
    if (target.length <= 42) return target;
    const sep = target.includes('\\') ? '\\' : '/';
    const parts = target.split(sep).filter(Boolean);
    if (parts.length >= 2) {
        const tail = parts.slice(-2).join(sep);
        if (tail.length <= 42) return '…' + sep + tail;
    }
    return '…' + target.slice(-39);
}

function applyActivity(payload) {
    if (!payload || !el.activityStrip) return;
    showActivityStrip();
    // Don't auto-start ticker here — it's driven by session_status.
    if (payload.role) {
        el.activityStrip.dataset.role = payload.role;
    }
    if (payload.kind) {
        el.activityStrip.dataset.kind = payload.kind;
    }
    setActivityActor(payload.actorLabel || payload.actor || 'Agents');
    const detail = payload.detail
        || (payload.tool ? `${payload.tool}${payload.target ? ' · ' + payload.target : ''}` : '')
        || 'Working…';
    setActivityDetail(detail);
    rememberActivityTool(payload);
    if (payload.tool) appendToolCallInChat(payload);
}

// ── Inline tool-call rows ─────────────────────────────────────────────────

function toolCallIcon(tool) {
    const t = String(tool || '').toLowerCase();
    const s = `viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"`;
    if (t.includes('read') || (t.includes('file') && !t.includes('write') && !t.includes('delet'))) {
        return `<svg ${s}><path d="M4 2h6l3 3v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z"/><path d="M10 2v4h3"/></svg>`;
    }
    if (t.includes('grep') || t.includes('search')) {
        return `<svg ${s}><circle cx="6.5" cy="6.5" r="3.5"/><path d="M10.5 10.5l3 3"/></svg>`;
    }
    if (t.includes('glob') || (t.includes('list') && !t.includes('guidance'))) {
        return `<svg ${s}><path d="M2 6a1 1 0 0 1 1-1h3.5L8 7h6v6a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1z"/></svg>`;
    }
    if (t.includes('guidance') || t.includes('guide')) {
        return `<svg ${s}><path d="M4 2h8a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1z"/><path d="M6 5h4M6 8h4M6 11h2"/></svg>`;
    }
    if (t.includes('write') || t.includes('creat') || t.includes('edit')) {
        return `<svg ${s}><path d="M11 2l3 3-8 8H3v-3z"/></svg>`;
    }
    if (t.includes('bash') || t.includes('shell') || t.includes('exec')) {
        return `<svg ${s}><rect x="2" y="3" width="12" height="10" rx="1.5"/><path d="M5 7.5l2 1.5-2 1.5"/><path d="M9.5 10.5h3"/></svg>`;
    }
    if (t.includes('delet') || t.includes('remov')) {
        return `<svg ${s}><path d="M3 5h10M6 5V3.5h4V5M5.5 5l.5 8h5l.5-8"/></svg>`;
    }
    return `<svg ${s}><circle cx="8" cy="8" r="2.5"/><path d="M8 2v1.5M8 12.5V14M2 8h1.5M12.5 8H14M3.9 3.9l1 1M11.1 11.1l1 1M11.1 3.9l-1 1M4.9 11.1l-1 1"/></svg>`;
}

function appendToolCallInChat(payload) {
    if (!payload || !payload.tool || !el.messagesList) return;

    const keyStr = [
        payload.actor || '',
        payload.ticketId || '',
        payload.tool || '',
        payload.target || '',
        payload.detail || '',
    ].join('\x00');
    let hash = 0;
    for (let i = 0; i < keyStr.length; i++) {
        hash = (Math.imul(31, hash) + keyStr.charCodeAt(i)) | 0;
    }
    const domId = 'tc-' + Math.abs(hash).toString(36);

    const status = payload.kind === 'tool_failed' ? 'failed'
        : payload.kind === 'tool_done' ? 'done'
        : 'running';
    const role = payload.role || 'system';

    let div = el.messagesList.querySelector(`.message.tool-call[data-tc-id="${CSS.escape(domId)}"]`);
    const isNew = !div;
    if (!div) {
        div = document.createElement('div');
        div.className = 'message tool-call';
        div.dataset.tcId = domId;
    }

    const icon = toolCallIcon(payload.tool);
    const target = payload.target ? escHtml(payload.target) : '';
    const detail = payload.detail || '';
    const actor = payload.actorLabel || payload.actor || '';
    const targetInDetail = target && detail.includes(payload.target);
    const showDetail = detail && detail !== payload.tool && !targetInDetail;

    div.innerHTML = `
        <div class="tool-call-row ${escHtml(status)}" data-role="${escHtml(role)}">
            <span class="tool-call-dot"></span>
            <span class="tool-call-icon">${icon}</span>
            <span class="tool-call-name">${escHtml(payload.tool)}</span>
            ${target ? `<span class="tool-call-sep">·</span><span class="tool-call-target" title="${target}">${target}</span>` : ''}
            ${showDetail ? `<span class="tool-call-sep">·</span><span class="tool-call-detail">${escHtml(detail)}</span>` : ''}
            <span class="tool-call-spacer"></span>
            ${actor ? `<span class="tool-call-actor">${escHtml(actor)}</span>` : ''}
        </div>
    `;

    if (isNew) {
        el.messagesList.appendChild(div);
        scrollToBottom();
    }
}

// ── Window controls ───────────────────────────────────────────────────────

el.minimizeBtn.addEventListener('click', () => {
    if (typeof window.runtime?.WindowMinimise === 'function') {
        window.runtime.WindowMinimise();
    }
});

el.closeBtn.addEventListener('click', () => {
    if (typeof window.runtime?.Quit === 'function') {
        window.runtime.Quit();
    }
});

// ── Session start ─────────────────────────────────────────────────────────

async function startSession() {
    const task = el.taskInput.value.trim();
    if (!task) return;
    const selectedMode = state.sessionMode || 'plan';

    el.sendBtn.classList.add('sending');
    el.sendBtn.addEventListener('animationend', () => el.sendBtn.classList.remove('sending'), { once: true });

    el.taskInput.value = '';
    resizeTextarea();

    appendMessage({
        id: `msg-${Date.now()}`,
        role: 'user',
        content: task,
        time: new Date().toISOString(),
        done: true,
    });

    hideEmptyState();
    setStatus('running', selectedMode === 'plan' ? 'Planning' : 'Running');

    const isFirst = !state.hasSpawned;
    if (selectedMode === 'build') {
        state.hasSpawned = true;
    }

    try {
        const session = await go.startSession(task, state.workDir, selectedMode);
        state.session = session;
        updateBranchLabel(session.branch);

        if (isFirst && selectedMode === 'build') {
            showSpawnAnimation(session.branch);
        }
    } catch (err) {
        appendMessage({
            id: `msg-${Date.now()}`,
            role: 'system',
            content: `**Session failed**\n\n${String(err || 'Unknown error')}`,
            time: new Date().toISOString(),
            done: true,
        });
        setStatus('error', 'Error');
    }
}

// ── Agent spawn animation (first session only) ────────────────────────────

async function showSpawnAnimation(branch) {
    const cmCount   = state.config.cmCount   || 1;
    const maxAgents = state.config.maxAgents || 4;
    const branchName = branch || 'codedone/work-…';

    const panel = document.createElement('div');
    panel.className = 'spawn-panel';
    panel.innerHTML = `
        <div class="spawn-panel-header">
            <div class="spawn-spinner" id="spawnSpinner"></div>
            <div class="spawn-header-text">
                <div class="spawn-header-title">Session initialising</div>
                <div class="spawn-header-status">
                    <div class="spawn-status-dot" id="spawnStatusDot"></div>
                    <span class="spawn-status-label" id="spawnStatusLabel">Spawning agents…</span>
                </div>
            </div>
        </div>
        <div class="spawn-body">
            <div class="spawn-section" id="spawnSectionCM">
                <div class="spawn-section-head">
                    <span class="spawn-section-title">Contre-Maître</span>
                    <span class="spawn-section-counter" id="spawnCounterCM">
                        <span class="count-current" id="spawnCurrCM">0</span><span style="color:rgba(255,255,255,0.2)"> / </span><span>${cmCount}</span>
                    </span>
                </div>
                <div class="spawn-dots-row" id="spawnDotsCM"></div>
            </div>
            <div class="spawn-section" id="spawnSectionImpl">
                <div class="spawn-section-head">
                    <span class="spawn-section-title">Implementers</span>
                    <span class="spawn-section-counter" id="spawnCounterImpl">
                        <span class="count-current" id="spawnCurrImpl">0</span><span style="color:rgba(255,255,255,0.2)"> / </span><span>${maxAgents}</span>
                    </span>
                </div>
                <div class="spawn-dots-row" id="spawnDotsImpl"></div>
            </div>
        </div>
        <div class="spawn-panel-footer" id="spawnFooter">
            <svg class="spawn-branch-icon" width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round">
                <circle cx="4" cy="4" r="1.5"/><circle cx="4" cy="12" r="1.5"/><circle cx="12" cy="4" r="1.5"/>
                <path d="M4 5.5v5M5.5 4h3a3 3 0 0 1 3 3v1.5"/>
            </svg>
            <span class="spawn-branch-name">${escHtml(branchName)}</span>
        </div>
    `;

    el.messagesList.appendChild(panel);
    scrollToBottom();

    // Contre-Maître row
    await sleep(130);
    panel.querySelector('#spawnSectionCM').classList.add('visible');

    const dotsCM    = panel.querySelector('#spawnDotsCM');
    const currCM    = panel.querySelector('#spawnCurrCM');
    const counterCM = panel.querySelector('#spawnCounterCM');
    counterCM.classList.add('ticking');

    for (let i = 0; i < cmCount; i++) {
        await sleep(160);
        const d = document.createElement('div');
        d.className = 'spawn-agent-dot cm';
        dotsCM.appendChild(d);
        currCM.textContent = i + 1;
        scrollToBottom();
    }
    counterCM.classList.remove('ticking');
    counterCM.classList.add('done');

    // Implementers row
    await sleep(260);
    panel.querySelector('#spawnSectionImpl').classList.add('visible');

    const dotsImpl    = panel.querySelector('#spawnDotsImpl');
    const currImpl    = panel.querySelector('#spawnCurrImpl');
    const counterImpl = panel.querySelector('#spawnCounterImpl');
    counterImpl.classList.add('ticking');

    for (let i = 0; i < maxAgents; i++) {
        await sleep(110);
        const d = document.createElement('div');
        d.className = 'spawn-agent-dot impl';
        dotsImpl.appendChild(d);
        currImpl.textContent = i + 1;
        scrollToBottom();
    }
    counterImpl.classList.remove('ticking');
    counterImpl.classList.add('done');

    // Footer + done state
    await sleep(200);
    panel.querySelector('#spawnFooter').classList.add('visible');
    scrollToBottom();

    await sleep(160);
    panel.querySelector('#spawnSpinner').classList.add('done');
    panel.querySelector('#spawnStatusDot').classList.add('done');
    panel.querySelector('#spawnStatusLabel').textContent = 'Ready';
    panel.querySelector('.spawn-header-title').textContent = 'Session initialised';
}

// Dev-mode mock flow
async function mockAgentFlow(task) {
    await sleep(400);
    appendMessage({
        id: `msg-${Date.now()}`,
        role: 'contre-maitre',
        content: `Received task: "${task}"\n\nBeginning intensive search phase — analysing codebase structure, dependencies, and determining implementation order...`,
        time: new Date().toISOString(),
        done: true,
    });
    scrollToBottom();

    await sleep(1200);
    appendMessage({
        id: `msg-${Date.now()}`,
        role: 'contre-maitre',
        content: `Search complete.\n\nFeature breakdown:\n  1. Session initialisation\n  2. Provider API integration\n  3. Contre-Maître loop\n  4. Implementer commit cycle\n  5. Finalizer validation\n\nIssuing feature 1 to Implementer...`,
        time: new Date().toISOString(),
        done: true,
    });
    scrollToBottom();

    await sleep(1800);
    appendMessage({
        id: `msg-${Date.now()}`,
        role: 'implementer',
        content: `Implementing feature 1: Session initialisation\n\n  + session.go\n  + config.go\n  ~ main.go (startup hook)\n\nCommitting to codedone/work branch...`,
        time: new Date().toISOString(),
        done: true,
    });
    scrollToBottom();

    await sleep(1400);
    appendMessage({
        id: `msg-${Date.now()}`,
        role: 'contre-maitre',
        content: `Reviewing diff...\n\nAll changes look correct. Proceeding to feature 2.`,
        time: new Date().toISOString(),
        done: true,
    });
    scrollToBottom();

    setStatus('idle', SESSION_MODES[state.sessionMode]?.statusIdle || 'Idle');
}

// ── Session reset (matrix dissolve) ──────────────────────────────────────

const MATRIX_CHARS = 'アイウエオカキクケコサシスセソタチツテトナニヌネノ0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZｱｲｳｴｵ!@#$&><[]{}';

function scrambleLine(original, progress) {
    const keepTo = Math.floor(original.length * (1 - progress));
    let out = original.slice(0, keepTo);
    for (let i = keepTo; i < original.length; i++) {
        const ch = original[i];
        out += (ch === ' ') ? ' ' : MATRIX_CHARS[Math.floor(Math.random() * MATRIX_CHARS.length)];
    }
    return out;
}

async function dissolveElement(elem) {
    elem.style.animation = 'none';
    const contentEl = elem.querySelector('.agent-card-content, .user-bubble-text');

    if (!contentEl) {
        elem.style.transition = 'opacity 0.22s ease, transform 0.22s ease';
        elem.style.opacity = '0';
        elem.style.transform = 'translateY(-5px)';
        await sleep(240);
        await collapseElement(elem);
        return;
    }

    const original = contentEl.textContent;
    elem.classList.add('matrix-dissolving');
    contentEl.style.color = '#00e546';
    contentEl.style.textShadow = '0 0 8px rgba(0, 229, 70, 0.55)';

    const TOTAL_MS  = 300;
    const FRAME_MS  = 28;
    const frames    = Math.ceil(TOTAL_MS / FRAME_MS);

    for (let f = 0; f < frames; f++) {
        const p = f / frames;
        const lines = original.split('\n');
        contentEl.textContent = lines.map(l => scrambleLine(l, Math.min(1, p * 1.4))).join('\n');
        await sleep(FRAME_MS);
    }

    elem.style.transition = 'opacity 0.18s ease, transform 0.18s ease';
    elem.style.opacity = '0';
    elem.style.transform = 'translateY(-6px) scale(0.985)';
    await sleep(200);
    await collapseElement(elem);
}

async function collapseElement(elem) {
    const h = elem.getBoundingClientRect().height;
    const cs = getComputedStyle(elem);
    const mt = parseFloat(cs.marginTop) || 0;
    const mb = parseFloat(cs.marginBottom) || 0;

    // Lock current dimensions, then transition to 0 so layout flow shrinks
    // bottom-up — the scroll container auto-clamps scrollTop, which makes
    // the viewport visibly track the dissolving wave upward.
    elem.style.height = `${h}px`;
    elem.style.marginTop = `${mt}px`;
    elem.style.marginBottom = `${mb}px`;
    elem.style.overflow = 'hidden';
    elem.style.flex = '0 0 auto';
    // Force reflow so the explicit values take before transitioning
    void elem.offsetHeight;
    elem.style.transition = 'height 0.22s ease, margin 0.22s ease, padding 0.22s ease';
    elem.style.height = '0px';
    elem.style.marginTop = '0px';
    elem.style.marginBottom = '0px';
    elem.style.paddingTop = '0px';
    elem.style.paddingBottom = '0px';
    await sleep(230);
}

async function resetSession() {
    const area = document.querySelector('.main-area');
    const items = Array.from(el.messagesList.children);
    try { await go.resetSession(); } catch (_) {}
    if (items.length === 0) {
        state.session = null;
        state.hasSpawned = false;
        updateBranchLabel('no branch');
        setStatus('idle', SESSION_MODES[state.sessionMode]?.statusIdle || 'Idle');
        stopActivityTicker();
        activityState.startedAt = 0;
        activityState.frozenMs = 0;
        activityState.lastDetail = '';
        hideActivityStrip();
        el.emptyState.classList.remove('hidden');
        applySessionMode(state.sessionMode);
        el.taskInput.focus();
        return;
    }

    el.newSessionBtn.classList.add('resetting');
    el.sendBtn.disabled = true;
    el.taskInput.disabled = true;

    // Pin viewport to the bottom of the (shrinking) content each frame so the
    // dissolving wave appears to scroll the view upward as items collapse.
    const prevBehavior = area.style.scrollBehavior;
    area.style.scrollBehavior = 'auto';
    let pinning = true;
    (function pin() {
        if (!pinning) return;
        area.scrollTop = area.scrollHeight - area.clientHeight;
        requestAnimationFrame(pin);
    })();

    // Fire dissolves bottom → top with stagger
    const STAGGER = 65;
    await Promise.all(
        [...items].reverse().map((item, i) => sleep(i * STAGGER).then(() => dissolveElement(item)))
    );

    pinning = false;
    area.style.scrollBehavior = prevBehavior;

    // Clear & reset state
    el.messagesList.innerHTML = '';
    state.session = null;
    state.hasSpawned = false;
    updateBranchLabel('no branch');
    setStatus('idle', SESSION_MODES[state.sessionMode]?.statusIdle || 'Idle');
    // Drop the live activity strip + clock so a fresh session starts at 0:00.
    stopActivityTicker();
    activityState.startedAt = 0;
    activityState.frozenMs = 0;
    activityState.lastDetail = '';
    hideActivityStrip();

    el.emptyState.classList.remove('hidden');
    applySessionMode(state.sessionMode);
    el.newSessionBtn.classList.remove('resetting');
    el.sendBtn.disabled = false;
    el.taskInput.disabled = false;
    el.taskInput.focus();
}

el.newSessionBtn.addEventListener('click', resetSession);

// ── Message rendering ─────────────────────────────────────────────────────

const ROLE_LABELS = {
    'contre-maitre': 'Contre-Maître',
    'implementer':   'Implementer',
    'finalizer':     'Finalizer',
    'system':        'System',
    'user':          'You',
};

function appendMessage(msg) {
    const role  = msg.role || 'system';
    const time  = new Date(msg.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    const label = msg.label || ROLE_LABELS[role] || role;
    const status = msg.status || (msg.done ? 'done' : 'running');
    const statusLabel = status === 'running' ? 'Running' : status === 'waiting' ? 'Waiting' : 'Done';
    const phase = msg.phase ? ` · ${msg.phase}` : '';
    const ticket = msg.ticketId ? ` · ${msg.ticketId}` : '';

    let div = el.messagesList.querySelector(`.message[data-id="${CSS.escape(String(msg.id))}"]`);
    const isNew = !div;
    if (!div) {
        div = document.createElement('div');
        div.dataset.id = msg.id;
    }
    div.className = `message ${role}`;
    div.dataset.status = status;

    if (msg.phase === 'file_diff') {
        renderFileDiffMessage(div, msg, time);
        if (isNew) el.messagesList.appendChild(div);
        return;
    }

    if (role === 'user') {
        div.innerHTML = `
            <div class="user-bubble">
                <div class="user-bubble-text markdown-body">${renderMarkdown(msg.content)}</div>
                <div class="user-bubble-footer">
                    <span class="user-bubble-you">You</span>
                    <span class="user-bubble-time">${time}</span>
                </div>
            </div>
        `;
    } else {
        // Implementer cards get a premium "ticket X / N" pill in the header
        // so the user always sees backlog progress without opening anything.
        const showTicketPill = role === 'implementer' && msg.ticketIndex && msg.ticketTotal;
        let ticketPillHtml = '';
        if (showTicketPill) {
            const idx = Number(msg.ticketIndex) || 0;
            const tot = Number(msg.ticketTotal) || 0;
            const pct = tot > 0 ? Math.max(0, Math.min(100, (idx / tot) * 100)) : 0;
            const idStr = msg.ticketId ? String(msg.ticketId) : '';
            const titleStr = msg.ticketTitle ? String(msg.ticketTitle) : '';
            const titleAttr = (idStr && titleStr) ? `${idStr} · ${titleStr}` : (titleStr || idStr || '');
            ticketPillHtml = `
                <div class="ticket-pill" title="${escHtml(titleAttr)}">
                    <svg class="ticket-pill-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M2.5 5.5h11v1.2a1 1 0 0 0 0 2v1.6a1 1 0 0 0 0 2v1.2h-11v-1.2a1 1 0 0 0 0-2V8.7a1 1 0 0 0 0-2z"/>
                        <line x1="6" y1="6.5" x2="6" y2="11.5" stroke-dasharray="1.4 1.6"/>
                    </svg>
                    <span class="ticket-pill-text">
                        <span class="ticket-pill-current">${idx}</span>
                        <span class="ticket-pill-sep">/</span>
                        <span class="ticket-pill-total">${tot}</span>
                    </span>
                    <span class="ticket-pill-track"><span class="ticket-pill-fill" style="width:${pct.toFixed(1)}%"></span></span>
                </div>
            `;
        }
        div.innerHTML = `
            <div class="agent-card${showTicketPill ? ' has-ticket-pill' : ''}">
                <div class="agent-card-bar"></div>
                <div class="agent-card-inner">
                    <div class="agent-card-header">
                        <span class="agent-name">${escHtml(label)}</span>
                        <span class="agent-meta-sep">·</span>
                        <span class="agent-status-text">${escHtml(statusLabel + phase + ticket)}</span>
                        ${ticketPillHtml}
                        <time class="agent-card-time">${time}</time>
                    </div>
                    <div class="agent-card-content markdown-body">${renderMarkdown(msg.content)}</div>
                </div>
            </div>
        `;
    }

    if (isNew) {
        el.messagesList.appendChild(div);
    }
    applyConsolePreferences(state.config);
}

function escHtml(str) {
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function renderMarkdown(content) {
    const raw = String(content ?? '');
    const md = window.marked;
    if (!md || typeof md.parse !== 'function') {
        return escHtml(raw).replace(/\n/g, '<br>');
    }
    try {
        const renderer = typeof md.Renderer === 'function' ? new md.Renderer() : null;
        if (renderer) {
            renderer.html = token => escHtml(token?.text ?? token?.raw ?? '');
        }
        const html = md.parse(raw, {
            gfm: true,
            breaks: true,
            mangle: false,
            headerIds: false,
            renderer,
        });
        return sanitizeMarkdownHtml(html);
    } catch (_) {
        return escHtml(raw).replace(/\n/g, '<br>');
    }
}

function sanitizeMarkdownHtml(html) {
    const template = document.createElement('template');
    template.innerHTML = String(html || '');
    const allowedTags = new Set([
        'P', 'BR', 'STRONG', 'EM', 'CODE', 'PRE', 'BLOCKQUOTE',
        'UL', 'OL', 'LI', 'A', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6',
        'HR', 'TABLE', 'THEAD', 'TBODY', 'TR', 'TH', 'TD', 'DEL',
        'INPUT',
    ]);
    const walk = node => {
        for (const child of Array.from(node.childNodes)) {
            if (child.nodeType === Node.ELEMENT_NODE) {
                if (!allowedTags.has(child.tagName)) {
                    child.replaceWith(document.createTextNode(child.textContent || ''));
                    continue;
                }
                sanitizeMarkdownElement(child);
                walk(child);
            } else if (child.nodeType !== Node.TEXT_NODE) {
                child.remove();
            }
        }
    };
    walk(template.content);
    return template.innerHTML;
}

function sanitizeMarkdownElement(elm) {
    for (const attr of Array.from(elm.attributes)) {
        const name = attr.name.toLowerCase();
        const value = attr.value || '';
        if (name.startsWith('on') || name === 'style') {
            elm.removeAttribute(attr.name);
            continue;
        }
        if (elm.tagName === 'A') {
            if (name === 'href') {
                const href = value.trim();
                if (!/^(https?:|mailto:|#)/i.test(href)) {
                    elm.removeAttribute(attr.name);
                }
                continue;
            }
            if (name === 'title') continue;
            elm.removeAttribute(attr.name);
            continue;
        }
        if (elm.tagName === 'CODE' && name === 'class' && /^language-[a-z0-9_-]+$/i.test(value)) {
            continue;
        }
        if (elm.tagName === 'INPUT') {
            if (name === 'type' && value.toLowerCase() === 'checkbox') continue;
            if (name === 'checked' || name === 'disabled') continue;
            elm.removeAttribute(attr.name);
            continue;
        }
        elm.removeAttribute(attr.name);
    }
    if (elm.tagName === 'A' && elm.getAttribute('href')) {
        elm.setAttribute('target', '_blank');
        elm.setAttribute('rel', 'noreferrer noopener');
    }
    if (elm.tagName === 'INPUT') {
        elm.setAttribute('disabled', 'disabled');
    }
}

const DIFF_OP_VERB = {
    edit:   'edited',
    write:  'created',
    delete: 'deleted',
};

const DIFF_OP_ICON = {
    edit:   '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M11 2l3 3-8 8H3v-3z"/></svg>',
    write:  '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 1.5h7l3 3V14a.5.5 0 0 1-.5.5h-9A.5.5 0 0 1 3 14z"/><path d="M10 1.5V5h3"/><path d="M8 8v4M6 10h4"/></svg>',
    delete: '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4h10M6 4V2.5h4V4M5 4l.5 9.5h5L11 4"/></svg>',
};

function renderFileDiffMessage(div, msg, timeStr) {
    let preview;
    try {
        preview = JSON.parse(msg.content || '{}');
    } catch (_) {
        div.innerHTML = `<div class="agent-card"><div class="agent-card-inner"><div class="agent-card-content markdown-body">${renderMarkdown(msg.content || '')}</div></div></div>`;
        return;
    }
    const op = preview.op || 'edit';
    const verb = DIFF_OP_VERB[op] || 'changed';
    const icon = DIFF_OP_ICON[op] || DIFF_OP_ICON.edit;
    const fullPath = preview.absPath || preview.path || '';
    const adds = preview.adds || 0;
    const removes = preview.removes || 0;
    const start = preview.lineStart || 0;
    const end = preview.lineEnd || 0;
    const span = (start && end) ? `L${start}${end !== start ? '–' + end : ''}` : '';

    const linesHtml = (preview.lines || []).map(l => {
        const t = l.type || 'context';
        const sym = t === 'add' ? '+' : t === 'remove' ? '−' : ' ';
        const num = t === 'add' ? (l.newNum ? String(l.newNum) : '') : (l.oldNum ? String(l.oldNum) : (l.newNum ? String(l.newNum) : ''));
        return `<div class="diff-line diff-${t}">`
             + `<span class="diff-num">${num}</span>`
             + `<span class="diff-sym">${sym}</span>`
             + `<span class="diff-text">${escHtml(l.text ?? '')}</span>`
             + `</div>`;
    }).join('');

    const moreHtml = preview.truncated && preview.hiddenMore
        ? `<div class="diff-more">··· ${preview.hiddenMore} more line${preview.hiddenMore === 1 ? '' : 's'}</div>`
        : '';

    const statHtml = [
        adds    ? `<span class="diff-stat diff-stat-add">+${adds}</span>`     : '',
        removes ? `<span class="diff-stat diff-stat-rem">−${removes}</span>` : '',
    ].filter(Boolean).join('');

    div.innerHTML = `
        <div class="diff-card diff-${op}">
            <div class="diff-card-header">
                <span class="diff-card-icon">${icon}</span>
                <span class="diff-card-verb">${verb}</span>
                <span class="diff-card-path" title="${escHtml(fullPath)}">${escHtml(fullPath)}</span>
                ${span ? `<span class="diff-card-span">${span}</span>` : ''}
                ${statHtml ? `<span class="diff-card-stats">${statHtml}</span>` : ''}
                <span class="diff-card-time">${timeStr}</span>
            </div>
            ${linesHtml ? `<div class="diff-card-body">${linesHtml}${moreHtml}</div>` : ''}
        </div>
    `;
}

// ── UI helpers ────────────────────────────────────────────────────────────

function hideEmptyState() {
    if (!el.emptyState.classList.contains('hidden')) {
        el.emptyState.classList.add('hidden');
    }
}

function setStatus(type, label) {
    el.statusDot.className = `status-dot ${type}`;
    el.statusLabel.textContent = label;
    applyInputLock(type);
}

// applyInputLock maps the high-level session status onto:
//   • the send-button mode (send | pause | resume), and
//   • whether the prompt input + workdir picker accept user input.
//
// Running  → button becomes a pause icon, input read-only.
// Paused   → button becomes a resume icon, input still read-only.
// Waiting  → input read-only (the questions overlay owns input here),
//            button stays in send mode but is disabled until we leave waiting.
// Anything else (idle/done/error) → button reverts to send, input unlocked.
function applyInputLock(status) {
    const btn = el.sendBtn;
    const prompt = document.querySelector('.input-prompt');
    const taskInput = el.taskInput;
    const workdir = el.inputWorkdir;
    const modeSwitch = el.modeSwitch;
    const modeCfg = SESSION_MODES[state.sessionMode] || SESSION_MODES.plan;
    if (!btn) return;

    const locked = status === 'running' || status === 'paused' || status === 'waiting';

    if (taskInput) {
        if (locked) {
            taskInput.setAttribute('readonly', 'readonly');
            taskInput.dataset._prevPlaceholder = taskInput.dataset._prevPlaceholder || taskInput.placeholder;
            if (status === 'paused')  taskInput.placeholder = 'Session paused — resume to continue';
            else if (status === 'running') taskInput.placeholder = 'Session running — input locked';
            else if (status === 'waiting') taskInput.placeholder = 'Waiting on your answers above';
            taskInput.blur();
        } else {
            taskInput.removeAttribute('readonly');
            if (taskInput.dataset._prevPlaceholder) {
                taskInput.placeholder = taskInput.dataset._prevPlaceholder;
                delete taskInput.dataset._prevPlaceholder;
            }
        }
    }
    if (taskInput) {
        if (status === 'running') {
            taskInput.placeholder = modeCfg.runningPlaceholder;
        } else if (!locked) {
            taskInput.placeholder = modeCfg.placeholder;
            delete taskInput.dataset._prevPlaceholder;
        }
    }
    if (prompt) prompt.classList.toggle('is-locked', locked);
    if (workdir) workdir.classList.toggle('is-locked', locked);
    if (modeSwitch) modeSwitch.classList.toggle('is-locked', locked);

    if (status === 'running') {
        btn.dataset.mode = 'pause';
        btn.disabled = false;
        btn.title = 'Pause session';
    } else if (status === 'paused') {
        btn.dataset.mode = 'resume';
        btn.disabled = false;
        btn.title = 'Resume session';
    } else if (status === 'waiting') {
        btn.dataset.mode = 'send';
        btn.disabled = true;
        btn.title = 'Awaiting answers';
    } else {
        btn.dataset.mode = 'send';
        btn.disabled = false;
        btn.title = 'Start session (Enter)';
    }
    if (btn.dataset.mode === 'send' && !btn.disabled) {
        btn.title = modeCfg.startTitle;
    }
}

// ── Pending questions overlay ────────────────────────────────────────────

let questionsState = {
    open: false,
    questions: [],
    textareas: [],
};

function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[c]);
}

function buildQuestionCard(q, index) {
    const card = document.createElement('div');
    card.className = 'question-card';
    card.style.animationDelay = `${60 + index * 80}ms`;

    const rationale = q.rationale
        ? `<div class="question-card-rationale"><span class="question-card-rationale-label">Why</span>${escapeHtml(q.rationale)}</div>`
        : '';

    card.innerHTML = `
        <div class="question-card-head">
            <div class="question-card-num">${index + 1}</div>
            <div class="question-card-prompt">${escapeHtml(q.prompt || '')}</div>
        </div>
        ${rationale}
        <textarea
            class="question-card-input"
            rows="2"
            spellcheck="false"
            placeholder="Type your answer…"
            data-qid="${escapeHtml(String(q.id ?? index))}"
        ></textarea>
    `;
    return card;
}

function autoGrowTextarea(ta) {
    ta.style.height = 'auto';
    ta.style.height = Math.min(ta.scrollHeight, 200) + 'px';
}

function refreshSubmitState() {
    const allFilled = questionsState.textareas.every(ta => ta.value.trim().length > 0);
    const btn = el.questionsSubmit;
    if (btn.classList.contains('is-loading') || btn.classList.contains('is-success')) return;
    btn.disabled = !allFilled;
}

function openQuestionsOverlay(payload) {
    if (questionsState.open) closeQuestionsOverlay({ silent: true });

    questionsState.open = true;
    questionsState.questions = payload.questions;
    questionsState.textareas = [];

    el.questionsHeaderSub.textContent = payload.summary
        || 'Contre-Maître needs a couple of details before planning can continue.';

    el.questionsBody.innerHTML = '';
    payload.questions.forEach((q, i) => {
        const card = buildQuestionCard(q, i);
        el.questionsBody.appendChild(card);
        const ta = card.querySelector('textarea');
        ta.addEventListener('input', () => { autoGrowTextarea(ta); refreshSubmitState(); });
        ta.addEventListener('keydown', e => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                e.preventDefault();
                submitQuestionsOverlay();
            }
        });
        questionsState.textareas.push(ta);
    });

    const btn = el.questionsSubmit;
    btn.classList.remove('is-loading', 'is-success');
    btn.querySelector('.qsubmit-label').textContent = 'Submit answers';
    btn.disabled = true;

    el.questionsOverlay.classList.remove('hidden', 'is-closing');
    requestAnimationFrame(() => {
        if (questionsState.textareas[0]) questionsState.textareas[0].focus();
    });
}

function closeQuestionsOverlay({ silent = false } = {}) {
    const overlay = el.questionsOverlay;
    if (!questionsState.open || overlay.classList.contains('hidden')) return;
    questionsState.open = false;
    overlay.classList.add('is-closing');
    setTimeout(() => {
        overlay.classList.remove('is-closing');
        overlay.classList.add('hidden');
        el.questionsBody.innerHTML = '';
        questionsState.textareas = [];
        questionsState.questions = [];
    }, 240);
    if (!silent) setStatus('waiting', 'Awaiting input');
}

async function submitQuestionsOverlay() {
    const btn = el.questionsSubmit;
    if (btn.disabled || btn.classList.contains('is-loading')) return;

    const answers = questionsState.textareas.map(ta => ta.value.trim());
    if (answers.some(a => a.length === 0)) {
        refreshSubmitState();
        return;
    }

    btn.classList.add('is-loading');
    btn.querySelector('.qsubmit-label').textContent = 'Sending…';
    btn.disabled = true;
    questionsState.textareas.forEach(ta => { ta.disabled = true; });

    try {
        await go.submitQuestionAnswers(answers);
        btn.classList.remove('is-loading');
        btn.classList.add('is-success');
        btn.querySelector('.qsubmit-label').textContent = 'Sent';
        setStatus('running', 'Running');
        setTimeout(() => closeQuestionsOverlay({ silent: true }), 380);
    } catch (err) {
        btn.classList.remove('is-loading');
        btn.querySelector('.qsubmit-label').textContent = 'Submit answers';
        questionsState.textareas.forEach(ta => { ta.disabled = false; });
        refreshSubmitState();
        appendMessage({
            id: `msg-${Date.now()}`,
            role: 'system',
            content: `Failed to submit question answers: ${err}`,
            time: new Date().toISOString(),
            done: true,
        });
        setStatus('waiting', 'Awaiting input');
    }
}

el.questionsSubmit.addEventListener('click', submitQuestionsOverlay);

el.questionsOverlay.addEventListener('click', e => {
    if (e.target === el.questionsOverlay) closeQuestionsOverlay();
});

document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && questionsState.open) {
        e.preventDefault();
        closeQuestionsOverlay();
        appendMessage({
            id: `msg-${Date.now()}`,
            role: 'system',
            content: 'Session is waiting for CM1 question answers.',
            time: new Date().toISOString(),
            done: true,
        });
    }
});

async function handlePendingQuestions(payload) {
    if (!payload || !Array.isArray(payload.questions) || payload.questions.length === 0) return;
    setStatus('waiting', 'Awaiting input');
    openQuestionsOverlay(payload);
}

function updateBranchLabel(branch) {
    const text = el.branchLabel.childNodes[2];
    if (text) text.textContent = ' ' + (branch && branch.trim() ? branch : 'no branch');
}

async function refreshBranchFromWorkdir() {
    try {
        const branch = await go.getCurrentBranch(state.workDir);
        updateBranchLabel(branch || 'no branch');
    } catch (_) {
        updateBranchLabel('no branch');
    }
}

// Returns just the last folder name from a path (used for the compact input-bar pill)
function workdirShort(p) {
    if (!p || p === '~/') return '~/';
    const sep = p.includes('\\') ? '\\' : '/';
    const parts = p.split(sep).filter(Boolean);
    return parts[parts.length - 1] || p;
}

function setPathLabel(dir) {
    const raw = dir || '~/';
    el.pathLabel.title = raw; // full path on hover
    const nodes = el.pathLabel.childNodes;
    for (let i = nodes.length - 1; i >= 0; i--) {
        if (nodes[i].nodeType === Node.TEXT_NODE) {
            nodes[i].textContent = ' ' + raw;
            return;
        }
    }
}

let _userAtBottom = true;
let _autoScrollUntil = 0;
let _autoScrollTimer = null;

function setupScrollTracking() {
    const area = document.querySelector('.main-area');
    if (!area) return;
    area.addEventListener('scroll', () => {
        const atBottom = area.scrollHeight - area.scrollTop - area.clientHeight <= 80;
        if (Date.now() < _autoScrollUntil && !atBottom) return;
        _userAtBottom = atBottom;
    });
    area.addEventListener('wheel', e => {
        if (e.deltaY < 0) {
            _autoScrollUntil = 0;
            _userAtBottom = false;
        }
    }, { passive: true });
    area.addEventListener('touchstart', () => {
        _autoScrollUntil = 0;
    }, { passive: true });
}

function scrollToBottom() {
    if (state.config.autoScroll === false) return;
    if (!_userAtBottom) return;
    const area = document.querySelector('.main-area');
    if (!area) return;

    const jump = () => {
        if (!_userAtBottom) return;
        _autoScrollUntil = Date.now() + 650;
        const prevBehavior = area.style.scrollBehavior;
        area.style.scrollBehavior = 'auto';
        area.scrollTop = area.scrollHeight;
        area.style.scrollBehavior = prevBehavior;
    };

    jump();
    requestAnimationFrame(() => {
        jump();
        requestAnimationFrame(jump);
    });

    clearTimeout(_autoScrollTimer);
    _autoScrollTimer = setTimeout(jump, 380);
}

function sleep(ms) {
    return new Promise(r => setTimeout(r, ms));
}

// ── Textarea auto-resize ──────────────────────────────────────────────────

function resizeTextarea() {
    el.taskInput.style.height = 'auto';
    el.taskInput.style.height = Math.min(el.taskInput.scrollHeight, 140) + 'px';
}

el.taskInput.addEventListener('input', resizeTextarea);

el.taskInput.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        // Enter only starts a session when the button is in send mode.
        // While running/paused, Enter is swallowed (the input is read-only
        // anyway, but this guards against IME-style edge cases).
        if (el.sendBtn.dataset.mode === 'send' && !el.sendBtn.disabled) {
            startSession();
        }
    }
});

// Single button, three behaviors driven by data-mode set by applyInputLock.
el.sendBtn.addEventListener('click', async () => {
    const mode = el.sendBtn.dataset.mode || 'send';
    if (el.sendBtn.disabled) return;
    if (mode === 'pause') {
        // Optimistic UI swap — backend will confirm with a session_status event.
        el.sendBtn.dataset.mode = 'resume';
        el.sendBtn.title = 'Resume session';
        try { await go.pauseSession(); } catch (_) {}
    } else if (mode === 'resume') {
        el.sendBtn.dataset.mode = 'pause';
        el.sendBtn.title = 'Pause session';
        try { await go.resumeSession(); } catch (_) {}
    } else {
        startSession();
    }
});

// ── Work directory picker ─────────────────────────────────────────────────

[el.modePlan, el.modeBuild].forEach(btn => {
    if (!btn) return;
    btn.addEventListener('click', () => {
        if (el.modeSwitch?.classList.contains('is-locked')) return;
        applySessionMode(btn.dataset.mode);
    });
});

el.inputWorkdir.addEventListener('click', async () => {
    if (typeof window.go?.main?.App?.PickDirectory !== 'function') return;
    try {
        const dir = await window.go.main.App.PickDirectory();
        if (dir) {
            state.workDir = dir;
            state.config = { ...state.config, lastWorkDir: dir };
            el.workdirLabel.textContent = workdirShort(dir);
            setPathLabel(dir);
            refreshBranchFromWorkdir();
        }
    } catch (_) {}
});

// ── Settings ──────────────────────────────────────────────────────────────

// CM power bar
function updateCmPower(level) {
    const group = $('cmCountGroup');
    if (!group) return;
    group.setAttribute('data-cm-level', String(level));
}
document.querySelectorAll('input[name="cmCount"]').forEach(radio => {
    radio.addEventListener('change', () => {
        if (radio.checked) updateCmPower(parseInt(radio.value));
    });
});

// Tab switching
document.querySelectorAll('.settings-tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
        const tab = btn.dataset.tab;
        const incoming = $('tab-' + tab);
        if (!incoming || incoming.classList.contains('pane-exiting')) return;
        if (!incoming.classList.contains('hidden')) return;

        const outgoing = document.querySelector('.settings-pane:not(.hidden)');

        document.querySelectorAll('.settings-tab-btn').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        $('settingsTabTitle').textContent = btn.textContent.trim();

        if (outgoing) {
            outgoing.classList.add('pane-exiting');
            setTimeout(() => {
                outgoing.classList.remove('pane-exiting');
                outgoing.classList.add('hidden');
                incoming.classList.remove('hidden');
                const settingsBody = document.querySelector('.settings-body');
                if (settingsBody) settingsBody.scrollTop = 0;
            }, 140);
        } else {
            incoming.classList.remove('hidden');
        }
    });
});

// Temperature controls
function updateTemperatureControls() {
    const enabled = $('enableTemperature').checked;
    $('temperature').disabled = !enabled;
    $('tempVal').textContent = enabled
        ? parseFloat($('temperature').value).toFixed(2)
        : 'Provider default';
}

$('enableTemperature').addEventListener('change', updateTemperatureControls);
$('temperature').addEventListener('input', updateTemperatureControls);

// Context window badge
const MODEL_CTX = {
    'deepseek-v4-flash': '1M',
    'deepseek-v4-pro': '1M',
    'deepseek/deepseek-chat': '128k',
    'anthropic/claude-sonnet-4.6': '1M',
    'openai/gpt-5.1': '400k',
    'gpt-5.4': '400k',
    'gpt-5.4-mini': '400k',
    'deepseek-chat': '128k', 'deepseek-v3': '128k', 'deepseek-reasoner': '128k',
    'claude-opus-4-7': '1M', 'claude-sonnet-4-6': '1M', 'claude-haiku-4-5': '200k',
    'claude-3-5-sonnet-20241022': '200k', 'claude-3-5-haiku-20241022': '200k',
    'gpt-4o': '128k', 'gpt-4o-mini': '128k', 'gpt-4-turbo': '128k',
    'o1': '200k', 'o3': '200k', 'o4-mini': '200k',
    'qwen2.5-72b-instruct': '128k', 'qwen2.5-coder-32b-instruct': '32k',
    'gemini-2.0-flash': '1M', 'gemini-1.5-pro': '2M',
};

function updateModelBadge(input, badge) {
    const ctx = MODEL_CTX[input.value.trim().toLowerCase()];
    if (ctx) {
        badge.textContent = ctx + ' ctx';
        badge.classList.remove('hidden');
    } else {
        badge.classList.add('hidden');
    }
}

el.cmModelInput.addEventListener('input', () => updateModelBadge(el.cmModelInput, $('cmCtxBadge')));
el.implementerModelInput.addEventListener('input', () => updateModelBadge(el.implementerModelInput, $('implCtxBadge')));
el.providerSelect.addEventListener('change', () => {
    const defaults = PROVIDER_DEFAULT_MODELS[el.providerSelect.value];
    if (!defaults) return;
    el.cmModelInput.value = defaults.cmModel;
    el.implementerModelInput.value = defaults.implementerModel;
    updateModelBadge(el.cmModelInput, $('cmCtxBadge'));
    updateModelBadge(el.implementerModelInput, $('implCtxBadge'));
});

function modelStatusText(cfg) {
    const cm = cfg.cmModel || cfg.model || 'none';
    const impl = cfg.implementerModel || cfg.model || 'none';
    return `cm: ${cm} / impl: ${impl}`;
}

el.settingsBtn.addEventListener('click', openSettings);
$('settingsClose').addEventListener('click', closeSettings);
el.settingsOverlay.addEventListener('click', e => {
    if (e.target === el.settingsOverlay) closeSettings();
});

// ── Unsaved changes tracking ──────────────────────────────────────────────
let _settingsSnapshot = null;

function takeSettingsSnapshot() {
    _settingsSnapshot = JSON.stringify(settingsFields());
    hideUnsavedBadge();
}

function checkUnsavedChanges() {
    if (!_settingsSnapshot) return;
    const isDirty = JSON.stringify(settingsFields()) !== _settingsSnapshot;
    const badge = $('settingsUnsavedBadge');
    if (!badge) return;
    if (isDirty && badge.classList.contains('hidden')) {
        badge.classList.remove('hidden', 'is-hiding');
    } else if (!isDirty && !badge.classList.contains('hidden')) {
        hideUnsavedBadge();
    }
}

function hideUnsavedBadge() {
    const badge = $('settingsUnsavedBadge');
    if (!badge || badge.classList.contains('hidden')) return;
    badge.classList.add('is-hiding');
    setTimeout(() => {
        badge.classList.add('hidden');
        badge.classList.remove('is-hiding');
    }, 220);
}

el.settingsOverlay.addEventListener('input', checkUnsavedChanges);
el.settingsOverlay.addEventListener('change', checkUnsavedChanges);

// ── Theme card picker — live preview ────────────────────────────────────────
document.querySelectorAll('.theme-card').forEach(btn => {
    btn.addEventListener('click', () => {
        document.querySelectorAll('.theme-card').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        applyTheme(btn.dataset.themeVal);
        checkUnsavedChanges();
    });
});

function settingsFields() {
    const cmModel = el.cmModelInput.value.trim();
    const implementerModel = el.implementerModelInput.value.trim();
    const maxTokens = Math.min(Math.max(parseInt($('maxTokens').value) || 8192, 256), 384000);
    return {
        // Agents
        provider:        $('providerSelect').value,
        model:           implementerModel || cmModel,
        cmModel,
        implementerModel,
        apiKey:          $('apiKeyInput').value.trim(),
        cmCount:         parseInt(document.querySelector('input[name="cmCount"]:checked')?.value || '1'),
        maxAgents:       parseInt($('maxAgents').value) || 4,
        agentTimeout:    parseInt($('agentTimeout').value) || 1200,
        maxTokens,
        enableTemperature: $('enableTemperature').checked,
        temperature:     parseFloat($('temperature').value),
        enableFinalizer: $('enableFinalizer').checked,
        // Git
        autoCreateBranch: $('autoCreateBranch').checked,
        requireCleanTree: $('requireCleanTree').checked,
        autoCommit:       $('autoCommit').checked,
        branchPrefix:     $('branchPrefix').value.trim() || 'codedone/work-',
        gitPath:          $('gitPath').value.trim(),
        // Console
        showTimestamps:   $('showTimestamps').checked,
        autoScroll:       $('autoScroll').checked,
        // Appearance
        theme: document.querySelector('.theme-card.active')?.dataset.themeVal || 'dark',
    };
}

function applySettingsToForm(cfg) {
    // Agents
    $('providerSelect').value    = cfg.provider || 'deepseek';
    const cmModel = cfg.cmModel || cfg.model || 'deepseek-v4-pro';
    const implementerModel = cfg.implementerModel || cfg.model || 'deepseek-v4-flash';
    el.cmModelInput.value        = cmModel;
    el.implementerModelInput.value = implementerModel;
    $('apiKeyInput').value       = cfg.apiKey || '';
    const cmEl = $('cmCount' + (cfg.cmCount || 1));
    if (cmEl) cmEl.checked = true;
    updateCmPower(cfg.cmCount || 1);
    $('maxAgents').value         = cfg.maxAgents || 4;
    $('agentTimeout').value      = cfg.agentTimeout || 1200;
    $('maxTokens').value         = cfg.maxTokens || 8192;
    $('enableTemperature').checked = !!cfg.enableTemperature;
    $('temperature').value       = cfg.temperature ?? 0.3;
    updateTemperatureControls();
    $('enableFinalizer').checked = cfg.enableFinalizer !== false;
    updateModelBadge(el.cmModelInput, $('cmCtxBadge'));
    updateModelBadge(el.implementerModelInput, $('implCtxBadge'));
    // Git
    $('autoCreateBranch').checked = cfg.autoCreateBranch !== false;
    $('requireCleanTree').checked = cfg.requireCleanTree || false;
    $('autoCommit').checked       = !!cfg.autoCommit;
    $('branchPrefix').value       = cfg.branchPrefix || 'codedone/work-';
    $('gitPath').value            = cfg.gitPath || '';
    // Console
    $('showTimestamps').checked = cfg.showTimestamps !== false;
    $('autoScroll').checked     = cfg.autoScroll !== false;
    // Appearance
    const theme = cfg.theme || 'dark';
    document.querySelectorAll('.theme-card').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.themeVal === theme);
    });
    applyConsolePreferences(cfg);
}

function applyConsolePreferences(cfg = state.config) {
    const show = cfg.showTimestamps !== false;
    document.body.classList.toggle('hide-message-times', !show);
    applyTheme(cfg.theme || 'dark');
}

function applyTheme(theme) {
    document.body.classList.add('theme-transitioning');
    if (theme === 'classy') {
        document.body.dataset.theme = 'classy';
    } else {
        delete document.body.dataset.theme;
    }
    setTimeout(() => document.body.classList.remove('theme-transitioning'), 600);
}

async function openSettings() {
    if (!el.settingsOverlay.classList.contains('hidden')) return;
    applySettingsToForm(state.config);
    takeSettingsSnapshot();
    setBodyState('settings-open', true);
    el.settingsOverlay.classList.remove('hidden');
    try {
        const cfg = await go.getConfig();
        state.config = cfg;
        if (!el.settingsOverlay.classList.contains('hidden')) {
            applySettingsToForm(cfg);
            takeSettingsSnapshot();
        }
    } catch (_) {}
}

function closeSettings() {
    const overlay = el.settingsOverlay;
    if (overlay.classList.contains('hidden') || overlay.classList.contains('is-closing')) return;
    applyTheme(state.config.theme || 'dark');
    overlay.classList.add('is-closing');
    setTimeout(() => {
        overlay.classList.remove('is-closing');
        overlay.classList.add('hidden');
        setBodyState('settings-open', false);
    }, 320);
}

// ── Backlog panel ─────────────────────────────────────────────────────────

function openBacklog() {
    const overlay = el.backlogOverlay;
    if (!overlay.classList.contains('hidden') && !overlay.classList.contains('is-closing')) return;
    overlay.classList.remove('is-closing');
    overlay.classList.add('is-opening');
    overlay.classList.remove('hidden');
    setTimeout(() => overlay.classList.remove('is-opening'), 500);
}

function closeBacklog() {
    const overlay = el.backlogOverlay;
    if (overlay.classList.contains('hidden') || overlay.classList.contains('is-closing')) return;
    overlay.classList.add('is-closing');
    setTimeout(() => {
        overlay.classList.remove('is-closing');
        overlay.classList.add('hidden');
    }, 440);
}

function renderBacklog(tickets) {
    const body = el.backlogBody;
    const empty = el.backlogEmpty;
    const count = el.backlogHeaderCount;
    const badge = el.backlogBadge;

    if (!tickets || tickets.length === 0) {
        body.innerHTML = '';
        body.appendChild(empty);
        empty.classList.remove('hidden');
        count.textContent = '';
        badge.classList.add('hidden');
        return;
    }

    empty.classList.add('hidden');

    const done   = tickets.filter(t => t.status === 'done').length;
    const inProg = tickets.filter(t => t.status === 'in_progress').length;
    const total  = tickets.length;
    count.textContent = done + ' / ' + total;

    const current = done + inProg;
    if (total > 0) {
        badge.textContent = current + '/' + total;
        badge.classList.remove('hidden');
        badge.classList.toggle('is-active', inProg > 0);
    } else {
        badge.classList.add('hidden');
    }

    const statusLabel = { todo: 'Todo', in_progress: 'In Progress', done: 'Done', blocked: 'Blocked', deferred: 'Deferred' };

    const existing = new Map();
    body.querySelectorAll('.ticket-card').forEach(el => existing.set(el.dataset.ticketId, el));

    const ordered = [];
    tickets.forEach((t, idx) => {
        let card = existing.get(t.id);
        if (!card) {
            card = document.createElement('div');
            card.className = 'ticket-card' + (t.parentId ? ' is-child' : '');
            card.dataset.ticketId = t.id;
            card.style.animationDelay = (idx * 40) + 'ms';
            card.innerHTML = `
                <div class="ticket-card-top">
                    <div class="ticket-card-meta">
                        <div class="ticket-card-row1">
                            <span class="ticket-id">${escHtml(t.id)}</span>
                            <span class="ticket-title">${escHtml(t.title)}</span>
                        </div>
                        <div class="ticket-card-row2">
                            <span class="ticket-status-badge ${t.status}">${statusLabel[t.status] || t.status}</span>
                            ${t.assignedLabel ? '<span class="ticket-assignee">' + escHtml(t.assignedLabel) + '</span>' : ''}
                        </div>
                    </div>
                    <svg class="ticket-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <polyline points="9 18 15 12 9 6"/>
                    </svg>
                </div>
                <div class="ticket-card-details">
                    ${t.summary ? `<div class="ticket-detail-section"><div class="ticket-detail-label">Summary</div><div class="ticket-detail-text">${escHtml(t.summary)}</div></div>` : ''}
                    ${t.acceptanceCriteria && t.acceptanceCriteria.length ? `<div class="ticket-detail-section"><div class="ticket-detail-label">Acceptance criteria</div><ul class="ticket-detail-list">${t.acceptanceCriteria.map(c => '<li>' + escHtml(c.replace(/^[\-\*•]\s*/, '')) + '</li>').join('')}</ul></div>` : ''}
                    ${t.constraints && t.constraints.length ? `<div class="ticket-detail-section"><div class="ticket-detail-label">Constraints</div><ul class="ticket-detail-list">${t.constraints.map(c => '<li>' + escHtml(c.replace(/^[\-\*•]\s*/, '')) + '</li>').join('')}</ul></div>` : ''}
                </div>`;
            card.querySelector('.ticket-card-top').addEventListener('click', () => {
                card.classList.toggle('expanded');
            });
        } else {
            const badge = card.querySelector('.ticket-status-badge');
            if (badge) {
                badge.className = 'ticket-status-badge ' + t.status;
                badge.textContent = statusLabel[t.status] || t.status;
            }
            const assigneeEl = card.querySelector('.ticket-assignee');
            if (assigneeEl && t.assignedLabel) assigneeEl.textContent = t.assignedLabel;
        }
        ordered.push(card);
    });

    body.innerHTML = '';
    ordered.forEach(c => body.appendChild(c));
}

el.backlogBtn.addEventListener('click', () => {
    if (!el.backlogOverlay.classList.contains('hidden')) {
        closeBacklog();
    } else {
        openBacklog();
    }
});
el.backlogClose.addEventListener('click', closeBacklog);
el.backlogOverlay.addEventListener('click', e => {
    if (e.target === el.backlogOverlay) closeBacklog();
});

let saveTimer = null;

$('settingsSave').addEventListener('click', async () => {
    const btn = $('settingsSave');
    const label = btn.querySelector('.save-btn-label');
    if (btn.classList.contains('is-loading') || btn.classList.contains('is-success')) return;

    // Loading state
    clearTimeout(saveTimer);
    btn.classList.add('is-loading');
    label.textContent = 'Saving…';

    try {
        const cfg = { ...state.config, ...settingsFields() };
        await go.saveConfig(cfg);
        // Re-fetch from Go so state.config reflects any server-side overrides
        // (e.g. lastWorkDir preserved by SaveConfig) and is always authoritative.
        const refreshed = await go.getConfig();
        state.config = refreshed || cfg;
        takeSettingsSnapshot();
        applyConsolePreferences(cfg);
        el.providerLabel.childNodes[2].textContent = ' ' + providerLabel(cfg.provider);
        el.modelLabel.textContent = modelStatusText(cfg);

        // Success state + panel shimmer
        btn.classList.remove('is-loading');
        btn.classList.add('is-success');
        label.textContent = 'Saved!';
        const panel = document.querySelector('.settings-panel');
        panel.classList.add('save-flash');
        setTimeout(() => panel.classList.remove('save-flash'), 700);

        saveTimer = setTimeout(() => {
            btn.classList.remove('is-success');
            label.textContent = 'Save changes';
        }, 2500);
    } catch (err) {
        btn.classList.remove('is-loading');
        label.textContent = 'Save changes';
        console.error('Save failed:', err);
    }
});

// ── Keyboard shortcuts ────────────────────────────────────────────────────

document.addEventListener('keydown', e => {
    if (e.key === 'Escape') {
        if (!el.settingsOverlay.classList.contains('hidden')) {
            closeSettings();
        }
    }
    if ((e.ctrlKey || e.metaKey) && e.key === ',') {
        e.preventDefault();
        openSettings();
    }
    // Block zoom shortcuts
    if ((e.ctrlKey || e.metaKey) && (e.key === '+' || e.key === '-' || e.key === '=' || e.key === '0')) {
        e.preventDefault();
    }
});

// Block Ctrl+wheel zoom
document.addEventListener('wheel', e => {
    if (e.ctrlKey) e.preventDefault();
}, { passive: false });

if (el.titlebar) {
    const beginWindowMove = e => {
        if (e.target.closest('.titlebar-btn')) return;
        setWindowMoving(true);
    };
    el.titlebar.addEventListener('pointerdown', beginWindowMove);
    el.titlebar.addEventListener('mousedown', beginWindowMove);
}

document.addEventListener('pointerup', () => setWindowMoving(false));
document.addEventListener('pointercancel', () => setWindowMoving(false));
window.addEventListener('blur', () => setWindowMoving(false));

// ── Custom select dropdowns ───────────────────────────────────────────────

function initCustomSelects() {
    const portal = document.createElement('div');
    portal.className = 'custom-select-portal';
    document.body.appendChild(portal);

    let openTrigger = null;
    let openDropdown = null;

    function closeAll() {
        if (openTrigger) openTrigger.classList.remove('is-open');
        if (openDropdown) openDropdown.classList.remove('is-open');
        openTrigger = null;
        openDropdown = null;
    }

    document.addEventListener('click', closeAll);
    document.addEventListener('keydown', e => { if (e.key === 'Escape') closeAll(); });

    document.querySelectorAll('.settings-select').forEach(select => {
        const trigger = document.createElement('div');
        trigger.className = 'custom-select-trigger';
        trigger.innerHTML = `
            <span class="custom-select-value"></span>
            <svg class="custom-select-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="6 9 12 15 18 9"/>
            </svg>`;

        const valueEl = trigger.querySelector('.custom-select-value');

        const dropdown = document.createElement('div');
        dropdown.className = 'custom-select-dropdown';

        Array.from(select.options).forEach(opt => {
            const item = document.createElement('div');
            item.className = 'custom-select-option';
            item.textContent = opt.text;
            item.dataset.value = opt.value;
            item.addEventListener('click', e => {
                e.stopPropagation();
                select.value = opt.value;
                select.dispatchEvent(new Event('change', { bubbles: true }));
                closeAll();
            });
            dropdown.appendChild(item);
        });

        portal.appendChild(dropdown);

        function syncDisplay() {
            const sel = select.options[select.selectedIndex];
            valueEl.textContent = sel ? sel.text : '';
            dropdown.querySelectorAll('.custom-select-option').forEach(item => {
                item.classList.toggle('is-selected', item.dataset.value === select.value);
            });
        }

        syncDisplay();

        // Intercept .value setter so applySettingsToForm keeps the UI in sync
        const proto = Object.getPrototypeOf(select);
        const desc = Object.getOwnPropertyDescriptor(proto, 'value');
        Object.defineProperty(select, 'value', {
            get() { return desc.get.call(this); },
            set(v) { desc.set.call(this, v); syncDisplay(); }
        });

        trigger.addEventListener('click', e => {
            e.stopPropagation();
            if (openDropdown === dropdown) { closeAll(); return; }
            closeAll();
            const r = trigger.getBoundingClientRect();
            dropdown.style.top    = `${r.bottom + 4}px`;
            dropdown.style.left   = `${r.left}px`;
            dropdown.style.width  = `${r.width}px`;
            dropdown.classList.add('is-open');
            trigger.classList.add('is-open');
            openTrigger = trigger;
            openDropdown = dropdown;
        });

        select.parentNode.insertBefore(trigger, select);
        select.style.display = 'none';
    });
}

// ── Init ──────────────────────────────────────────────────────────────────

async function init() {
    initCustomSelects();
    setupScrollTracking();
    setupWailsEvents();
    applySessionMode(state.sessionMode);
    // Seed form immediately from JS defaults so settings open instantly correct
    applySettingsToForm(state.config);
    try {
        const [cfg, wd, tickets] = await Promise.all([go.getConfig(), go.getWorkDir(), go.getTickets()]);
        state.config = cfg;
        applySettingsToForm(cfg);
        if (tickets && tickets.length) {
            state.tickets = tickets;
            renderBacklog(tickets);
        }
        el.providerLabel.childNodes[2].textContent = ' ' + providerLabel(cfg.provider);
        el.modelLabel.textContent = modelStatusText(cfg);
        if (wd) {
            state.workDir = wd;
            el.workdirLabel.textContent = workdirShort(wd);
            setPathLabel(wd);
        } else {
            setPathLabel(state.workDir);
        }
        refreshBranchFromWorkdir();
    } catch (_) {
        setPathLabel(state.workDir);
        refreshBranchFromWorkdir();
    }
    el.taskInput.focus();

    // Catch external branch changes (e.g. user runs `git checkout` in a terminal
    // or the agent moves HEAD) by re-checking when the window regains focus.
    window.addEventListener('focus', () => { refreshBranchFromWorkdir(); });
}

init();
