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
    rendered: {},          // message_id -> true, dedupes POST responses vs SSE
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
    ':host{all:initial}' +
    '.kw-launch{position:fixed;right:20px;bottom:20px;width:56px;height:56px;border-radius:50%;border:0;' +
    'background:#1a1a2e;color:#fff;font-size:24px;cursor:pointer;box-shadow:0 4px 14px rgba(0,0,0,.25);z-index:2147483000}' +
    '.kw-panel{position:fixed;right:20px;bottom:88px;width:340px;max-width:calc(100vw - 40px);height:480px;max-height:calc(100vh - 120px);' +
    'display:none;flex-direction:column;background:#fff;border-radius:14px;box-shadow:0 12px 40px rgba(0,0,0,.28);overflow:hidden;' +
    'font:14px/1.45 system-ui,-apple-system,sans-serif;color:#1a1a2e;z-index:2147483000}' +
    '.kw-panel.open{display:flex}' +
    '.kw-head{padding:12px 16px;background:#1a1a2e;color:#fff;font-weight:600;display:flex;justify-content:space-between;align-items:center}' +
    '.kw-head button{border:0;background:none;color:#fff;font-size:18px;cursor:pointer}' +
    '.kw-log{flex:1;overflow-y:auto;padding:12px;display:flex;flex-direction:column;gap:8px;background:#f5f6fa}' +
    '.kw-msg{max-width:85%;padding:8px 12px;border-radius:12px;white-space:pre-wrap;word-wrap:break-word}' +
    '.kw-user{align-self:flex-end;background:#1a1a2e;color:#fff;border-bottom-right-radius:4px}' +
    '.kw-agent{align-self:flex-start;background:#fff;border:1px solid #e2e4ee;border-bottom-left-radius:4px}' +
    '.kw-note{align-self:center;color:#7a7d92;font-size:12px;text-align:center}' +
    '.kw-card{align-self:stretch;background:#fff;border:1px solid #d9dcf0;border-radius:12px;padding:12px}' +
    '.kw-card h4{margin:0 0 8px;font-size:13px}' +
    '.kw-card table{width:100%;border-collapse:collapse;font-size:13px}' +
    '.kw-card td{padding:2px 0}.kw-card td:first-child{color:#7a7d92;padding-right:10px;white-space:nowrap}' +
    '.kw-card button{margin-top:10px;width:100%;padding:8px;border:0;border-radius:8px;background:#2c6e49;color:#fff;font-weight:600;cursor:pointer}' +
    '.kw-card button:disabled{opacity:.5;cursor:default}' +
    '.kw-form{display:flex;flex-direction:column;gap:8px;padding:16px}' +
    '.kw-form input{padding:9px 10px;border:1px solid #d9dcf0;border-radius:8px;font:inherit}' +
    '.kw-form button{padding:10px;border:0;border-radius:8px;background:#1a1a2e;color:#fff;font-weight:600;cursor:pointer}' +
    '.kw-error{color:#b3261e;font-size:12px;min-height:14px}' +
    '.kw-input{display:flex;gap:8px;padding:10px;border-top:1px solid #e2e4ee;background:#fff}' +
    '.kw-input input{flex:1;padding:9px 10px;border:1px solid #d9dcf0;border-radius:8px;font:inherit}' +
    '.kw-input button{padding:0 14px;border:0;border-radius:8px;background:#1a1a2e;color:#fff;cursor:pointer}' +
    '.kw-input button:disabled{opacity:.5;cursor:default}' +
    '</style>' +
    '<button class="kw-launch" type="button" aria-label="Open chat">&#128172;</button>' +
    '<div class="kw-panel" role="dialog" aria-label="' + TITLE + ' chat">' +
    '  <div class="kw-head"><span></span><button type="button" aria-label="Close">&times;</button></div>' +
    '  <div class="kw-body" style="display:flex;flex-direction:column;flex:1;min-height:0"></div>' +
    '</div>';

  var launch = root.querySelector(".kw-launch");
  var panel = root.querySelector(".kw-panel");
  var body = root.querySelector(".kw-body");
  root.querySelector(".kw-head span").textContent = TITLE;
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
      '<div>Leave your name and email and ask for an appointment.</div>' +
      '<input name="name" placeholder="Your name" maxlength="200" required>' +
      '<input name="email" type="email" placeholder="Email" maxlength="254" required>' +
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
        addNote("You're connected. Ask for an appointment — for example: “I'd like a haircut Thursday evening”.");
        connectStream();
      }).catch(function (failure) {
        error.textContent = failure.message;
      });
    });
  }

  var log, input, sendButton;

  function renderChat() {
    body.innerHTML =
      '<div class="kw-log"></div>' +
      '<form class="kw-input"><input placeholder="Type a message…" maxlength="2000" autocomplete="off">' +
      '<button type="submit">Send</button></form>';
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

  function addConfirmationCard(confirmation) {
    var card = document.createElement("div");
    card.className = "kw-card";
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
      confirm.textContent = "Confirming…";
      sendMessage("Yes, confirm");
    });
    card.appendChild(confirm);
    log.appendChild(card);
    log.scrollTop = log.scrollHeight;
  }

  /* ---------------------------- conversation ---------------------------- */

  function renderTurn(turn) {
    if (turn.message_id && state.rendered[turn.message_id]) return;
    if (turn.message_id) state.rendered[turn.message_id] = true;
    addBubble("kw-agent", turn.message);
    if (turn.pending_confirmation) addConfirmationCard(turn.pending_confirmation);
    if (turn.outcome === "escalated") addNote("A person now handles this conversation.");
  }

  function sendMessage(text) {
    state.busy = true;
    if (sendButton) sendButton.disabled = true;
    addBubble("kw-user", text);
    var thinking = addBubble("kw-agent", "…");
    fetch(API + "/api/v1/demo/conversations/" + encodeURIComponent(state.conversationId) + "/messages", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": "Bearer " + state.token
      },
      body: JSON.stringify({ text: text, client_message_id: randomId() })
    }).then(function (response) {
      if (response.status === 503) throw new Error("The assistant is busy — please try again in a moment.");
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
