/*
 * Kontor embeddable chat widget.
 *
 * One script tag, no dependencies:
 *   <script src="https://your-kontor-host/widget/v1/kontor.js" defer></script>
 *
 * Optional attributes:
 *   data-kontor-url="https://your-kontor-host"  API origin (defaults to the
 *                                               origin the script was loaded from)
 *   data-kontor-title="Salon Nord"              panel title
 *
 * The widget talks to the demo conversation API with a per-conversation
 * bearer capability held in sessionStorage, and follows the durable SSE
 * stream with fetch-based streaming so the token never appears in a URL.
 */
(function () {
  "use strict";

  var script = document.currentScript;
  if (!script) return;
  var API = (script.getAttribute("data-kontor-url") || new URL(script.src).origin).replace(/\/+$/, "");
  var TITLE = script.getAttribute("data-kontor-title") || "Kontor";
  var STORE_KEY = "kontor-widget-v1:" + API;

  var state = {
    conversationId: null,
    token: null,
    lastEventId: 0,
    rendered: {},
    streaming: false,
    streamAbort: null,
    busy: false
  };

  try {
    var saved = JSON.parse(sessionStorage.getItem(STORE_KEY) || "null");
    if (saved && saved.conversationId && saved.token) {
      state.conversationId = saved.conversationId;
      state.token = saved.token;
      state.lastEventId = saved.lastEventId || 0;
    }
  } catch (_) { /* corrupted storage is treated as a fresh session */ }

  function persist() {
    try {
      sessionStorage.setItem(STORE_KEY, JSON.stringify({
        conversationId: state.conversationId,
        token: state.token,
        lastEventId: state.lastEventId
      }));
    } catch (_) { /* private mode: the session simply won't survive reloads */ }
  }

  /* ---------- DOM (inside a shadow root so host CSS cannot leak in) ---------- */

  var host = document.createElement("div");
  var root = host.attachShadow({ mode: "closed" });
  root.innerHTML =
    '<style>' +
    ':host{all:initial;font-family:\"Geist\",ui-sans-serif,system-ui,-apple-system,\"Segoe UI\",sans-serif;' +
    '--surface-1:#10141b;--surface-2:#141922;--surface-3:#1a2029;--surface-inset:#0d1117;' +
    '--border-default:#29313d;--border-subtle:#1e2530;' +
    '--text-primary:#edf0f4;--text-secondary:#a4adba;--text-tertiary:#767f8e;' +
    '--accent:#6e78f0;--accent-hover:#8a92ff;' +
    '--green-500:#3fb95a;--status-error-fg:#f47883;--status-warning-fg:#edc257;' +
    '--shadow-overlay:0 8px 24px -6px rgba(3,6,12,0.5),0 2px 6px rgba(3,6,12,0.35);' +
    '--radius-xl:14px;--radius-lg:10px;--radius-md:7px;' +
    '--fs-body:15px;--fs-body-sm:13px;--fs-caption:12px;--fs-micro:11px;' +
    '--font-mono:\"Geist Mono\",ui-monospace,\"SF Mono\",\"JetBrains Mono\",monospace;' +
    '--ease-out:cubic-bezier(0.22,1,0.36,1);' +
    '--focus-ring:0 0 0 2px color-mix(in oklab,#6e78f0 45%,transparent)}' +

    '*,*::before,*::after{box-sizing:border-box}' +

    '.kw-launch{position:fixed;right:20px;bottom:20px;width:56px;height:56px;border-radius:50%;border:0;' +
    'background:var(--accent);color:#fff;cursor:pointer;box-shadow:var(--shadow-overlay);z-index:2147483000;' +
    'display:flex;align-items:center;justify-content:center;transition:background 150ms var(--ease-out),transform 150ms var(--ease-out);padding:0}' +
    '.kw-launch:hover{background:var(--accent-hover);transform:scale(1.05)}' +
    '.kw-launch:focus-visible{outline:none;box-shadow:var(--focus-ring),var(--shadow-overlay)}' +
    '.kw-launch svg{width:26px;height:26px;fill:none;stroke:#fff;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}' +

    '.kw-panel{position:fixed;right:20px;bottom:88px;width:380px;max-width:calc(100vw - 40px);height:520px;max-height:calc(100vh - 120px);' +
    'display:none;flex-direction:column;background:var(--surface-1);border:1px solid var(--border-default);border-radius:var(--radius-xl);' +
    'box-shadow:var(--shadow-overlay);overflow:hidden;font:var(--fs-body)/1.45 \"Geist\",ui-sans-serif,system-ui,-apple-system,\"Segoe UI\",sans-serif;' +
    'color:var(--text-primary);z-index:2147483000}' +
    '.kw-panel.open{display:flex}' +

    '.kw-head{padding:12px 16px;background:var(--surface-2);border-bottom:1px solid var(--border-subtle);display:flex;justify-content:space-between;align-items:center}' +
    '.kw-head-left{display:flex;align-items:center;gap:10px}' +
    '.kw-head-icon{width:28px;height:28px;border-radius:var(--radius-md);background:var(--accent);display:flex;align-items:center;justify-content:center;flex-shrink:0}' +
    '.kw-head-icon svg{width:15px;height:15px;fill:none;stroke:#fff;stroke-width:1.8;stroke-linecap:round;stroke-linejoin:round}' +
    '.kw-head-info{display:flex;flex-direction:column;gap:1px}' +
    '.kw-head-title{font-size:var(--fs-body-sm);font-weight:600;color:var(--text-primary);line-height:1.2}' +
    '.kw-head-sub{font-size:var(--fs-micro);color:var(--text-tertiary);display:flex;align-items:center;gap:5px;line-height:1.2}' +
    '.kw-head-dot{width:6px;height:6px;border-radius:50%;background:var(--green-500);flex-shrink:0}' +
    '.kw-head button{border:0;background:none;color:var(--text-tertiary);font-size:20px;cursor:pointer;padding:4px;border-radius:var(--radius-md);' +
    'line-height:1;transition:color 150ms var(--ease-out),background 150ms var(--ease-out)}' +
    '.kw-head button:hover{color:var(--accent);background:var(--surface-3)}' +
    '.kw-head button:focus-visible{outline:none;box-shadow:var(--focus-ring)}' +

    '.kw-log{flex:1;overflow-y:auto;padding:14px;display:flex;flex-direction:column;gap:14px;background:var(--surface-1);' +
    'scrollbar-width:thin;scrollbar-color:var(--border-default) transparent}' +
    '.kw-log[role=\"log\"]{scroll-behavior:smooth}' +

    '.kw-msg{padding:10px 14px;white-space:pre-wrap;word-wrap:break-word;font-size:var(--fs-body);line-height:1.45}' +
    '.kw-user{align-self:flex-end;background:var(--accent);color:#fff;border-radius:12px 12px 3px 12px;max-width:82%}' +
    '.kw-agent{align-self:flex-start;background:var(--surface-3);border:1px solid var(--border-subtle);color:var(--text-primary);border-radius:12px 12px 12px 3px;max-width:88%}' +

    '.kw-note{align-self:center;color:var(--text-tertiary);font-size:var(--fs-caption);text-align:center;padding:4px 0}' +

    '.kw-working{align-self:stretch;border:1px solid color-mix(in srgb,var(--accent) 40%,transparent);border-radius:var(--radius-lg);overflow:hidden;background:color-mix(in srgb,var(--accent) 7%,var(--surface-1))}' +
    '.kw-working-main{display:flex;align-items:center;gap:9px;padding:11px 13px 9px;font-size:var(--fs-body-sm);font-weight:600}' +
    '.kw-working-dot{width:10px;height:10px;border-radius:50%;background:var(--accent);animation:kw-pulse 1.3s ease-in-out infinite}' +
    '.kw-working-sub{padding:0 13px 11px;color:var(--text-secondary);font-size:var(--fs-caption)}' +
    '.kw-working-progress{height:3px;background:var(--surface-2);overflow:hidden}.kw-working-progress::after{content:"";display:block;width:36%;height:100%;background:var(--accent);animation:kw-progress 1.4s cubic-bezier(.4,0,.2,1) infinite}' +
    '@keyframes kw-pulse{0%,100%{opacity:.35;transform:scale(.8)}50%{opacity:1;transform:scale(1)}}@keyframes kw-progress{0%{transform:translateX(-100%)}100%{transform:translateX(280%)}}' +

    '.kw-card{align-self:stretch;background:var(--surface-2);border:1px solid var(--border-default);border-radius:var(--radius-lg);padding:14px}' +
    '.kw-confirm-banner{display:flex;align-items:center;gap:7px;margin:-14px -14px 13px;padding:9px 14px;background:color-mix(in srgb,var(--accent) 10%,var(--surface-1));border-bottom:1px solid color-mix(in srgb,var(--accent) 40%,transparent);color:var(--accent);font-size:var(--fs-micro);font-weight:600;letter-spacing:.08em;text-transform:uppercase}' +
    '.kw-card h4{margin:0 0 10px;font-size:var(--fs-body-sm);font-weight:600;color:var(--text-primary)}' +
    '.kw-card table{width:100%;border-collapse:collapse;font-size:var(--fs-body-sm)}' +
    '.kw-card td{padding:3px 0}.kw-card td:first-child{color:var(--text-tertiary);padding-right:12px;white-space:nowrap}' +
    '.kw-card td:last-child{color:var(--text-primary)}' +
    '.kw-card button{margin-top:12px;width:100%;padding:10px;border:0;border-radius:var(--radius-md);background:var(--green-500);' +
    'color:#fff;font-weight:600;font-size:var(--fs-body-sm);cursor:pointer;transition:opacity 150ms var(--ease-out)}' +
    '.kw-card button:hover{opacity:0.9}' +
    '.kw-card button:disabled{opacity:0.5;cursor:default}' +
    '.kw-card button:focus-visible{outline:none;box-shadow:var(--focus-ring)}' +

    '.kw-slot-picker{align-self:stretch;border:1px solid var(--border-default);border-radius:var(--radius-lg);overflow:hidden;background:var(--surface-2)}' +
    '.kw-slot-picker h4{margin:0;padding:11px 13px;border-bottom:1px solid var(--border-subtle);font-size:var(--fs-body-sm)}' +
    '.kw-slot-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(92px,1fr));gap:8px;padding:12px}' +
    '.kw-slot{min-height:44px;border:1px solid var(--border-default);border-radius:var(--radius-md);background:var(--surface-1);color:var(--text-primary);font:600 var(--fs-body-sm)/1 var(--font-mono);cursor:pointer}' +
    '.kw-slot:hover{border-color:var(--accent);background:var(--accent-subtle)}.kw-slot:focus-visible{outline:none;box-shadow:var(--focus-ring)}' +

    '.kw-escalation{align-self:stretch;display:flex;flex-direction:column;align-items:center;gap:8px;padding:14px 10px;border-top:1px solid color-mix(in srgb,var(--status-warning-fg) 45%,transparent);border-bottom:1px solid color-mix(in srgb,var(--status-warning-fg) 45%,transparent)}' +
    '.kw-escalation-icon{width:24px;height:24px;color:var(--status-warning-fg)}' +
    '.kw-escalation-icon svg{width:24px;height:24px;fill:none;stroke:currentColor;stroke-width:1.6;stroke-linecap:round;stroke-linejoin:round}' +
    '.kw-escalation-text{font-size:var(--fs-body-sm);color:var(--text-secondary);text-align:center}' +

    '.kw-escalation-restart{margin-top:10px;padding:8px 16px;border:1px solid var(--border-default);border-radius:var(--radius-md);' +
    'background:var(--surface-2);color:var(--text-primary);font-size:var(--fs-body-sm);font-weight:500;cursor:pointer;' +
    'transition:border-color 150ms var(--ease-out),background 150ms var(--ease-out)}' +
    '.kw-escalation-restart:hover{border-color:var(--accent);background:var(--surface-3)}' +
    '.kw-escalation-restart:focus-visible{outline:none;box-shadow:var(--focus-ring)}' +

    '.kw-form{display:flex;flex-direction:column;gap:10px;padding:20px 16px;justify-content:center;flex:1}' +
    '.kw-form-label{font-size:var(--fs-body-sm);color:var(--text-secondary);text-align:center;margin-bottom:4px}' +
    '.kw-form input{padding:10px 12px;border:1px solid var(--border-default);border-radius:var(--radius-md);' +
    'background:var(--surface-inset);color:var(--text-primary);font:var(--fs-body)/1.4 inherit;transition:border-color 150ms var(--ease-out)}' +
    '.kw-form input::placeholder{color:var(--text-tertiary)}' +
    '.kw-form input:focus{outline:none;border-color:var(--accent);box-shadow:var(--focus-ring)}' +
    '.kw-form button{padding:11px;border:0;border-radius:var(--radius-md);background:var(--accent);color:#fff;' +
    'font-weight:600;font-size:var(--fs-body-sm);cursor:pointer;transition:background 150ms var(--ease-out)}' +
    '.kw-form button:hover{background:var(--accent-hover)}' +
    '.kw-form button:focus-visible{outline:none;box-shadow:var(--focus-ring)}' +
    '.kw-error{color:var(--status-error-fg);font-size:var(--fs-caption);min-height:16px}' +

    '.kw-input{display:flex;gap:8px;padding:10px 12px;border-top:1px solid var(--border-subtle);background:var(--surface-inset);align-items:center}' +
    '.kw-input input{flex:1;padding:10px 12px;border:1px solid var(--border-default);border-radius:var(--radius-md);' +
    'background:var(--surface-inset);color:var(--text-primary);font:var(--fs-body)/1.4 inherit;transition:border-color 150ms var(--ease-out)}' +
    '.kw-input input::placeholder{color:var(--text-tertiary)}' +
    '.kw-input input:focus{outline:none;border-color:var(--accent);box-shadow:var(--focus-ring)}' +
    '.kw-input button{width:36px;height:36px;border-radius:50%;border:0;background:var(--accent);color:#fff;cursor:pointer;' +
    'display:flex;align-items:center;justify-content:center;flex-shrink:0;padding:0;transition:background 150ms var(--ease-out)}' +
    '.kw-input button:hover{background:var(--accent-hover)}' +
    '.kw-input button:disabled{opacity:0.4;cursor:default;background:var(--accent)}' +
    '.kw-input button:focus-visible{outline:none;box-shadow:var(--focus-ring)}' +
    '.kw-input button svg{width:16px;height:16px;fill:none;stroke:#fff;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}' +
    '</style>' +

    '<button class="kw-launch" type="button" aria-label="Open chat">' +
    '<svg viewBox="0 0 24 24"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/></svg>' +
    '</button>' +
    '<div class="kw-panel" role="dialog" aria-label="' + TITLE + ' chat">' +
    '  <div class="kw-head">' +
    '    <div class="kw-head-left">' +
    '      <div class="kw-head-icon"><svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg></div>' +
    '      <div class="kw-head-info"><span class="kw-head-title"></span><span class="kw-head-sub"><span class="kw-head-dot"></span>Books into your calendar</span></div>' +
    '    </div>' +
    '    <button type="button" aria-label="Close">&times;</button>' +
    '  </div>' +
    '  <div class="kw-body" style="display:flex;flex-direction:column;flex:1;min-height:0"></div>' +
    '</div>';

  var launch = root.querySelector(".kw-launch");
  var panel = root.querySelector(".kw-panel");
  var body = root.querySelector(".kw-body");
  root.querySelector(".kw-head-title").textContent = TITLE;
  root.querySelector(".kw-head button").addEventListener("click", function () {
    panel.classList.remove("open");
  });
  launch.addEventListener("click", function () {
    panel.classList.toggle("open");
    if (panel.classList.contains("open")) boot();
  });
  document.addEventListener("DOMContentLoaded", function () { document.body.appendChild(host); });
  if (document.readyState !== "loading") document.body.appendChild(host);

  /* ------------------------------ screens ------------------------------ */

  function boot() {
    if (state.conversationId && state.token) {
      renderChat();
      connectStream();
      return;
    }
    renderStartForm();
  }

  function renderStartForm() {
    body.innerHTML =
      '<form class="kw-form">' +
      '<div class="kw-form-label">Leave your name and ask for an appointment. We will ask for contact details only when needed.</div>' +
      '<input name="name" placeholder="Your name" maxlength="200" required aria-label="Your name">' +
      '<input name="email" type="email" placeholder="Email (optional)" maxlength="254" aria-label="Email (optional)">' +
      '<div class="kw-error"></div>' +
      '<button type="submit">Start chat</button>' +
      '</form>';
    var form = body.querySelector("form");
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      var error = form.querySelector(".kw-error");
      error.textContent = "";
      fetch(API + "/api/v1/demo/conversations", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          display_name: form.elements.name.value.trim(),
          email: form.elements.email.value.trim()
        })
      }).then(function (response) {
        if (!response.ok) throw new Error("The chat could not be started (" + response.status + ")");
        return response.json();
      }).then(function (created) {
        state.conversationId = created.conversation_id;
        state.token = created.capability_token;
        state.lastEventId = 0;
        state.rendered = {};
        persist();
        renderChat();
        addNote("You're connected. Ask for an appointment.");
        connectStream();
      }).catch(function (failure) {
        error.textContent = failure.message;
      });
    });
  }

  var log, input, sendButton;

  function renderChat() {
    body.innerHTML =
      '<div class="kw-log" role="log" aria-live="polite"></div>' +
      '<form class="kw-input"><input placeholder="Type a message\u2026" maxlength="2000" autocomplete="off" aria-label="Message Kontor">' +
      '<button type="submit" aria-label="Send message"><svg viewBox="0 0 24 24"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg></button></form>';
    log = body.querySelector(".kw-log");
    var form = body.querySelector("form");
    input = form.querySelector("input");
    sendButton = form.querySelector("button");
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      var text = input.value.trim();
      if (!text || state.busy) return;
      input.value = "";
      sendMessage(text);
    });
  }

  function addBubble(className, text) {
    var bubble = document.createElement("div");
    bubble.className = "kw-msg " + className;
    bubble.textContent = text;
    log.appendChild(bubble);
    log.scrollTop = log.scrollHeight;
    return bubble;
  }

  function addNote(text) {
    var note = document.createElement("div");
    note.className = "kw-note";
    note.textContent = text;
    log.appendChild(note);
    log.scrollTop = log.scrollHeight;
  }

  // The customer sees that Kontor is performing a server-side turn, rather
  // than an ambiguous typing indicator. It never claims a booking exists.
  function addWorkingIndicator() {
    var block = document.createElement("div");
    block.className = "kw-working";
    block.setAttribute("role", "status");
    block.setAttribute("aria-live", "polite");
    block.innerHTML =
      '<div class="kw-working-main"><span class="kw-working-dot" aria-hidden="true"></span>Kontor is working</div>' +
      '<div class="kw-working-sub">Checking your request safely before making any change.</div>' +
      '<div class="kw-working-progress" aria-hidden="true"></div>';
    log.appendChild(block);
    log.scrollTop = log.scrollHeight;
    return block;
  }

  function addEscalationNotice() {
    var block = document.createElement("div");
    block.className = "kw-escalation";
    block.innerHTML =
      '<div class="kw-escalation-icon"><svg viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg></div>' +
      '<div class="kw-escalation-text">A person now handles this conversation</div>';
    var restart = document.createElement("button");
    restart.type = "button";
    restart.className = "kw-escalation-restart";
    restart.textContent = "Start a new conversation";
    restart.addEventListener("click", function () { resetAndRestart(); });
    block.appendChild(restart);
    log.appendChild(block);
    log.scrollTop = log.scrollHeight;
  }

  function addConfirmationCard(confirmation) {
    var card = document.createElement("div");
    card.className = "kw-card";
	card.setAttribute("data-pending-confirmation", "true");
    var banner = document.createElement("div");
    banner.className = "kw-confirm-banner";
    banner.textContent = "Kontor will book this";
    card.appendChild(banner);
    var title = document.createElement("h4");
    title.textContent = confirmation.title || "Confirm this action";
    card.appendChild(title);
    var table = document.createElement("table");
    (confirmation.facts || []).forEach(function (fact) {
      var row = table.insertRow();
      row.insertCell().textContent = fact.label;
      row.insertCell().textContent = fact.value;
    });
    card.appendChild(table);
    var confirm = document.createElement("button");
    confirm.type = "button";
    confirm.textContent = "Confirm";
    confirm.addEventListener("click", function () {
      confirm.disabled = true;
      confirm.textContent = "Confirming\u2026";
      sendMessage("Yes, confirm");
    });
    card.appendChild(confirm);
    var note = document.createElement("div");
    note.className = "kw-note";
    note.style.marginTop = "8px";
    note.textContent = "Nothing is booked until you confirm.";
    card.appendChild(note);
    log.appendChild(card);
    log.scrollTop = log.scrollHeight;
  }

  /* ---------------------------- conversation ---------------------------- */

  function renderTurn(turn) {
    if (turn.message_id && state.rendered[turn.message_id]) return;
    if (turn.message_id) state.rendered[turn.message_id] = true;
    if (typeof turn.pending_confirmation_active === "boolean" && !turn.pending_confirmation_active) {
      log.querySelectorAll('[data-pending-confirmation="true"]').forEach(function (card) { card.remove(); });
    }
    addBubble("kw-agent", turn.message);
    if (turn.pending_confirmation) addConfirmationCard(turn.pending_confirmation);
    if (turn.outcome === "escalated" || turn.outcome === "budget_exhausted") addEscalationNotice();
  }

  function sendMessage(text) {
    state.busy = true;
    if (sendButton) sendButton.disabled = true;
    addBubble("kw-user", text);
    var thinking = addWorkingIndicator();
    fetch(API + "/api/v1/demo/conversations/" + encodeURIComponent(state.conversationId) + "/messages", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": "Bearer " + state.token
      },
      body: JSON.stringify({ text: text, client_message_id: randomId() })
    }).then(function (response) {
      if (response.status === 503) throw new Error("The assistant is busy \u2014 please try again in a moment.");
      if (!response.ok) throw new Error("The message could not be processed (" + response.status + ")");
      return response.json();
    }).then(function (turn) {
      thinking.remove();
      renderTurn(turn);
    }).catch(function (failure) {
      thinking.remove();
      addNote(failure.message);
    }).finally(function () {
      state.busy = false;
      if (sendButton) sendButton.disabled = false;
    });
  }

  /* ------------------------------- reset -------------------------------- */

  function resetAndRestart() {
    if (state.streamAbort) { state.streamAbort.abort(); state.streamAbort = null; }
    state.streaming = false;
    state.conversationId = null;
    state.token = null;
    state.lastEventId = 0;
    state.rendered = {};
    state.busy = false;
    try { sessionStorage.removeItem(STORE_KEY); } catch (_) {}
    renderStartForm();
  }

  /* ------------------------------- stream ------------------------------- */
  /* fetch-based SSE so the bearer capability travels as a header, never in
     the URL. Reconnects with backoff and resumes from Last-Event-ID. */

  function connectStream() {
    if (state.streaming || !state.conversationId) return;
    state.streaming = true;
    streamOnce(0);
  }

  function streamOnce(attempt) {
    var controller = new AbortController();
    state.streamAbort = controller;
    fetch(API + "/api/v1/demo/conversations/" + encodeURIComponent(state.conversationId) + "/events", {
      headers: {
        "Authorization": "Bearer " + state.token,
        "Last-Event-ID": String(state.lastEventId)
      },
      signal: controller.signal
    }).then(function (response) {
      if (!response.ok || !response.body) throw new Error("stream unavailable");
      attempt = 0;
      var reader = response.body.getReader();
      var decoder = new TextDecoder();
      var buffer = "";
      function pump() {
        return reader.read().then(function (chunk) {
          if (chunk.done) throw new Error("stream closed");
          buffer += decoder.decode(chunk.value, { stream: true });
          var frames = buffer.split("\n\n");
          buffer = frames.pop();
          frames.forEach(handleFrame);
          return pump();
        });
      }
      return pump();
    }).catch(function () {
      var delay = Math.min(30000, 1000 * Math.pow(2, attempt)) + Math.floor(Math.random() * 500);
      setTimeout(function () { streamOnce(attempt + 1); }, delay);
    });
  }

  function handleFrame(frame) {
    var id = null, event = "message", data = "";
    frame.split("\n").forEach(function (line) {
      if (line.indexOf("id:") === 0) id = parseInt(line.slice(3).trim(), 10);
      else if (line.indexOf("event:") === 0) event = line.slice(6).trim();
      else if (line.indexOf("data:") === 0) data += line.slice(5).trim();
    });
    if (id === null || isNaN(id) || id <= state.lastEventId) return;
    state.lastEventId = id;
    persist();
    if (event !== "turn_completed" || !log) return;
    try {
      renderTurn(JSON.parse(data));
    } catch (_) { /* a malformed frame is skipped; replay will not repeat it */ }
  }

  function randomId() {
    if (window.crypto && crypto.randomUUID) return crypto.randomUUID();
    return "kw-" + Date.now() + "-" + Math.random().toString(36).slice(2, 10);
  }
})();
