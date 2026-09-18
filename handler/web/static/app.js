(function () {
  "use strict";

  var POLL_MS = 1000, MAX_LOG = 200;
  var PILL = {AVAILABLE: "ok", PREPARING: "warn", CHARGING: "busy", FINISHING: "idle", FAULTED: "bad"};
  // what a person would call each state; the technical names are shown with Simulator controls on
  var PLAIN = {AVAILABLE: "Free", PREPARING: "Waiting", CHARGING: "Charging", FINISHING: "Done", FAULTED: "Out of order"};
  var COLOR = {AVAILABLE: "var(--ok)", PREPARING: "var(--warn)", CHARGING: "var(--busy)", FINISHING: "var(--muted)", FAULTED: "var(--bad)"};
  var TOLD = {AVAILABLE: "AVAILABLE", PREPARING: "BLOCKED", CHARGING: "CHARGING", FINISHING: "BLOCKED", FAULTED: "OUTOFORDER"};
  var DOT = {AVAILABLE: "var(--ok)", BLOCKED: "var(--warn)", CHARGING: "var(--busy)", OUTOFORDER: "var(--bad)"};
  var CABLE = "M56 56 C 78 104, 100 100, 120 66";

  var cpo = null, emsp = null, lastSequence = 0, messageCount = 0;
  var editingChargerID = null, worldID = null;
  var done = {plugged: false, started: false, billed: false};

  function $(id) { return document.getElementById(id); }
  function esc(value) {
    return String(value == null ? "" : value).replace(/[&<>"']/g, function (c) {
      return {"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"}[c];
    });
  }
  // "EVSE-000002" is "Charger 2" to a person; anything else keeps its own name
  function chargerName(id) {
    var match = /^EVSE-0*(\d+)$/.exec(id);
    return match ? "Charger " + match[1] : id;
  }
  function kwh(value) { return (value || 0).toFixed(1) + " kWh"; }
  function money(value) { return "$" + (value || 0).toFixed(2); }
  function clockTime(iso, withSeconds) {
    var options = {hour: "2-digit", minute: "2-digit", hour12: false};
    if (withSeconds) options.second = "2-digit";
    return new Date(iso).toLocaleTimeString([], options);
  }
  function setHTML(element, html) {
    if (element.__html !== html) { element.innerHTML = html; element.__html = html; }
  }
  function flash(message) {
    $("flash").textContent = message || "";
    if (message) setTimeout(function () { if ($("flash").textContent === message) $("flash").textContent = ""; }, 6000);
  }

  function call(method, url, body) {
    return fetch(url, {
      method: method,
      headers: body === undefined ? {} : {"Content-Type": "application/json"},
      body: body === undefined ? undefined : JSON.stringify(body)
    }).then(function (response) {
      if (response.status === 204) return null;
      return response.json().then(function (data) {
        if (!response.ok) throw new Error(data && data.error ? data.error : "HTTP " + response.status);
        return data;
      });
    });
  }
  function act(promise) {
    return promise.then(function () { flash(""); return poll(); }, function (error) { flash(error.message); });
  }

  // ---------- charger illustration ----------
  function draw(state, plugged) {
    var o = '<svg viewBox="0 0 220 108" role="img" aria-label="Charger ' + state.toLowerCase() + (plugged ? ", car plugged in" : ", no car") + '" style="--st:' + COLOR[state] + '">';
    o += '<line class="il-ground" x1="6" y1="98" x2="214" y2="98"></line>';
    if (plugged) {
      o += '<path class="il-body" d="M126 60 L142 40 H182 L198 60 H204 Q212 60 212 68 V80 Q212 84 208 84 H116 Q112 84 112 80 V68 Q112 60 120 60 Z"></path>';
      o += '<path class="il-glass" d="M133 60 L145 45 H160 V60 Z M165 45 H180 L191 60 H165 Z"></path>';
      o += '<circle class="il-wheel" cx="136" cy="86" r="10"></circle><circle class="il-hub" cx="136" cy="86" r="4"></circle>';
      o += '<circle class="il-wheel" cx="190" cy="86" r="10"></circle><circle class="il-hub" cx="190" cy="86" r="4"></circle>';
    }
    o += '<rect class="il-body" x="22" y="20" width="34" height="78" rx="5"></rect>';
    o += '<rect class="il-state" x="28" y="27" width="22" height="15" rx="2"></rect>';
    o += '<path class="il-line" d="M41 50 L35 62 H40 L37 72 L45 58 H40 Z"></path>';
    if (plugged) {
      o += '<path class="il-cable" d="' + CABLE + '"></path>';
      if (state === "CHARGING") o += '<path class="il-flow" d="' + CABLE + '"></path>';
      o += '<circle class="il-state" cx="120" cy="66" r="4"></circle>';
    } else {
      o += '<path class="il-cable" d="M56 56 C 72 60, 74 86, 60 88"></path><rect class="il-body" x="54" y="83" width="9" height="10" rx="2"></rect>';
    }
    if (state === "CHARGING") o += '<path class="il-state" d="M166 8 L156 24 H163 L160 35 L172 18 H165 Z"></path>';
    if (state === "FINISHING") o += '<path class="il-state-line" d="M154 22 L162 30 L176 12"></path>';
    if (state === "PREPARING") o += '<path class="il-state-line" d="M158 22 h0.1 M166 22 h0.1 M174 22 h0.1"></path>';
    if (state === "FAULTED") {
      o += '<path class="il-state" d="M39 2 L50 18 H28 Z"></path><path d="M39 8 V13 M39 15.5 v0.1" stroke="var(--surface)" stroke-width="2" stroke-linecap="round" fill="none"></path>';
      if (plugged) o += '<rect class="il-state" x="84" y="84" width="12" height="10" rx="2"></rect><path class="il-state-line" d="M86.5 84 V80 a3.5 3.5 0 0 1 7 0 V84"></path>';
    }
    return o + "</svg>";
  }

  // ---------- simulated chargers ----------
  function lastEnergy(chargerID) {
    var sessions = (cpo.sessions || []).filter(function (s) { return s.charger_id === chargerID && s.state === "COMPLETED"; });
    return sessions.length ? sessions[sessions.length - 1].energy_delivered_kwh : 0;
  }
  function metaLine(charger) {
    var soc = charger.live_state_of_charge == null ? "" : "battery " + Math.round(charger.live_state_of_charge * 100) + "%";
    switch (charger.state) {
      case "CHARGING":
        var active = charger.active_session || {};
        return "Charging at " + Math.round(active.power_kw || 0) + " kW · " + soc + " · " + kwh(active.energy_delivered_kwh) + " so far";
      case "PREPARING": return "Car plugged in (" + soc + "). Now start it from the phone.";
      case "FINISHING": return "Done: " + kwh(lastEnergy(charger.charger_id)) + " delivered, " + soc + ". Unplug to free the charger.";
      case "FAULTED": return charger.vehicle && charger.connector_locked ? "Broken, and the cable is stuck in the car" : "Broken";
    }
    return "Nobody here. Plug a car in.";
  }
  function actionButton(charger) {
    var actions = {
      AVAILABLE: ["plug-in", "Plug in", "primary"], PREPARING: ["unplug", "Unplug", ""],
      CHARGING: ["press-stop", "Press stop", ""], FINISHING: ["unplug", "Unplug", "primary"],
      FAULTED: ["clear-fault", "Fix it", "primary"]
    }[charger.state];
    return '<button class="btn ' + actions[2] + '" data-action="' + actions[0] + '">' + actions[1] + "</button>";
  }
  function behaviorLine(charger) {
    var labels = (charger.behaviors || []).map(function (spec) {
      var info = (cpo.behaviors || []).filter(function (b) { return b.kind === spec.kind; })[0];
      return info ? info.label : spec.kind;
    });
    var odd = (charger.behaviors || []).some(function (spec) { return spec.kind !== "realistic_reliability"; });
    var text = labels.length ? "This charger " + labels.join(", and ") : "This charger never fails";
    if (odd) text = "Set up to misbehave: " + labels.join(", and ");
    return '<div class="behaves' + (odd ? " odd" : " adv") + '"><span>' + esc(text) + '</span><button class="linkish adv" data-edit>' +
      (editingChargerID === charger.charger_id ? "Close" : "Change") + "</button></div>";
  }
  function editor(charger) {
    var site = (cpo.sites || []).filter(function (s) { return s.site_id === charger.site_id; })[0] || {};
    var o = '<div class="editor"><div class="facts">' + esc(site.name || charger.site_id) + " · " + charger.max_power_kw + " kW · " + money(charger.price_per_kwh) + "/kWh</div>";
    (cpo.behaviors || []).forEach(function (info) {
      var current = (charger.behaviors || []).filter(function (spec) { return spec.kind === info.kind; })[0];
      var params = Object.assign({}, info.default_params || {}, current && current.params || {});
      o += '<label class="opt"><input type="checkbox" data-kind="' + esc(info.kind) + '"' + (current ? " checked" : "") + "><span><b>" + esc(info.label) + '</b></span><span class="desc">' + esc(info.description) + "</span>";
      var names = Object.keys(params);
      if (names.length) {
        o += '<span class="params">';
        names.forEach(function (name) {
          o += "<label>" + esc(name) + ' <input type="number" step="any" min="0" data-param="' + esc(name) + '" value="' + esc(params[name]) + '"></label>';
        });
        o += "</span>";
      }
      o += "</label>";
    });
    o += '<div class="btns"><button class="btn primary small" data-save>Save</button>' +
      '<button class="btn danger small" data-action="inject-fault"' + (charger.state === "FAULTED" ? " disabled" : "") + ">Break it now</button>" +
      '<button class="btn danger small" data-remove' + (charger.session_id ? " disabled" : "") + ">Remove charger</button></div></div>";
    return o;
  }

  function renderChargers() {
    var container = $("chargers"), seen = {};
    cpo.chargers.forEach(function (charger) {
      var id = charger.charger_id;
      seen[id] = true;
      var card = container.querySelector('[data-id="' + id + '"]');
      if (!card) {
        card = document.createElement("div");
        card.className = "charger";
        card.dataset.id = id;
        card.innerHTML = '<div class="row"></div><div class="illus"></div><div class="meta"></div><div class="btns"></div><div class="slot-behaves"></div><div class="slot-editor"></div>';
        container.appendChild(card);
      }
      var parts = card.children;
      setHTML(parts[0], '<span class="name">' + esc(chargerName(id)) + "<small>" + charger.max_power_kw + ' kW</small><small class="id adv">' + esc(id) + "</small></span>" +
        '<span class="pill ' + PILL[charger.state] + '">' + PLAIN[charger.state] + '<span class="adv"> · ' + charger.state + "</span></span>");
      setHTML(parts[1], draw(charger.state, !!charger.vehicle));
      var meta = metaLine(charger);
      setHTML(parts[2], esc(meta));
      setHTML(parts[3], actionButton(charger));
      setHTML(parts[4], behaviorLine(charger));
      // an open editor holds the user's unsaved edits: only redraw it when it opens or closes
      var open = editingChargerID === id;
      if (open !== !!card.__editorOpen) {
        parts[5].innerHTML = open ? editor(charger) : "";
        card.__editorOpen = open;
      }
    });
    Array.prototype.slice.call(container.children).forEach(function (card) {
      if (!seen[card.dataset.id]) container.removeChild(card);
    });
    var count = cpo.chargers.length;
    $("charger-count").textContent = count + (count === 1 ? " charger" : " chargers");
  }

  $("chargers").addEventListener("click", function (event) {
    var card = event.target.closest(".charger");
    if (!card) return;
    var id = card.dataset.id, base = "/api/chargers/" + encodeURIComponent(id);
    if (event.target.dataset.action) {
      act(call("POST", base + "/actions/" + event.target.dataset.action));
    } else if (event.target.hasAttribute("data-edit")) {
      editingChargerID = editingChargerID === id ? null : id;
      renderChargers();
    } else if (event.target.hasAttribute("data-remove")) {
      editingChargerID = null;
      act(call("DELETE", base));
    } else if (event.target.hasAttribute("data-save")) {
      var specs = [];
      card.querySelectorAll("input[data-kind]").forEach(function (box) {
        if (!box.checked) return;
        var params = {};
        box.closest(".opt").querySelectorAll("input[data-param]").forEach(function (input) {
          params[input.dataset.param] = parseFloat(input.value) || 0;
        });
        specs.push({kind: box.dataset.kind, params: params});
      });
      editingChargerID = null;
      act(call("PUT", base + "/behaviors", specs));
    }
  });

  $("add-toggle").addEventListener("click", function () {
    setHTML($("add-site"), (cpo.sites || []).map(function (site) {
      return '<option value="' + esc(site.site_id) + '">' + esc(site.name) + "</option>";
    }).join(""));
    $("add-form").hidden = false;
    $("add-toggle").hidden = true;
  });
  function closeAddForm() { $("add-form").hidden = true; $("add-toggle").hidden = false; $("add-form").reset(); }
  $("add-cancel").addEventListener("click", closeAddForm);
  $("add-form").addEventListener("submit", function (event) {
    event.preventDefault();
    // leaving a field empty lets the simulator apply its own defaults, including default behaviors
    var input = {site_id: $("add-site").value};
    if ($("add-power").value) input.max_power_kw = parseFloat($("add-power").value);
    if ($("add-price").value) input.price_per_kwh = parseFloat($("add-price").value);
    act(call("POST", "/api/chargers", input)).then(closeAddForm);
  });

  function setAdvanced(on) {
    document.body.classList.toggle("advanced", on);
    $("advanced-toggle").checked = on;
    try { localStorage.setItem("advanced", on ? "1" : ""); } catch (error) { /* private window: not remembered */ }
  }
  $("advanced-toggle").addEventListener("change", function (event) { setAdvanced(event.target.checked); });
  try { setAdvanced(localStorage.getItem("advanced") === "1"); } catch (error) { setAdvanced(false); }

  $("reset").addEventListener("click", function () { act(call("POST", "/api/reset")); });

  document.querySelectorAll("[data-speed]").forEach(function (button) {
    button.addEventListener("click", function () { act(call("PUT", "/api/clock", {speed: parseFloat(button.dataset.speed)})); });
  });

  // ---------- driver's phone: only ever shows what the eMSP believes ----------
  function mySession() {
    var active = (emsp.sessions || []).filter(function (s) { return s.status === "ACTIVE"; });
    return active.length ? active[active.length - 1] : null;
  }
  function lastCommand() { return emsp.commands.length ? emsp.commands[emsp.commands.length - 1] : null; }
  function startPending() {
    var command = lastCommand();
    return !!command && command.kind === "START_SESSION" && command.result === "PENDING";
  }

  function renderPhone() {
    var session = mySession(), command = lastCommand(), o;
    if (session) {
      var dash = (Math.min(1, session.kwh / 60) * 163.4).toFixed(1);
      var stopping = command && command.kind === "STOP_SESSION" && command.result === "PENDING";
      o = '<div class="gauge"><svg viewBox="0 0 64 64" role="img" aria-label="Charging"><circle class="ring-bg" cx="32" cy="32" r="26"></circle>' +
        '<circle class="ring-fg" cx="32" cy="32" r="26" stroke-dasharray="' + dash + ' 163.4" transform="rotate(-90 32 32)"></circle>' +
        '<path class="ring-bolt" d="M35 16 L23 35 H31 L28 48 L41 28 H33 Z"></path></svg>' +
        '<div><div class="big">' + kwh(session.kwh) + '</div><div class="meta">Charging at ' + esc(chargerName(session.evse_uid)) + " · " + money(session.total_cost && session.total_cost.excl_vat) + "</div></div></div>" +
        '<div class="btns" style="margin-top:10px"><button class="btn danger" data-stop="' + esc(session.id) + '"' + (stopping ? " disabled" : "") + ">" + (stopping ? "Stopping…" : "Stop charging") + "</button></div>";
    } else if (startPending()) {
      o = '<div class="meta">Asking ' + esc(chargerName(command.evse_uid)) + " to start… plug the car in if you haven't.</div>";
    } else if (command && command.kind === "START_SESSION" && command.result !== "ACCEPTED") {
      o = '<div class="meta bad">Couldn\'t start ' + esc(chargerName(command.evse_uid)) + (command.message ? ": " + esc(command.message) : "") + '<span class="adv"> (' + esc(command.result) + ")</span></div>";
    } else {
      o = '<div class="empty">Not charging. Pick a charger below.</div>';
    }
    setHTML($("my-session"), o);

    var busy = !!session || startPending(), rows = "";
    (emsp.locations || []).forEach(function (location) {
      (location.evses || []).forEach(function (evse) {
        var right;
        if (session && session.evse_uid === evse.uid) right = '<span class="meta">in use (you)</span>';
        else if (evse.status === "CHARGING") right = '<span class="meta">in use</span>';
        else if (evse.status === "OUTOFORDER") right = '<span class="meta">out of order</span>';
        else right = '<button class="btn primary small" data-start="' + esc(evse.uid) + '" data-location="' + esc(location.id) + '"' + (busy ? " disabled" : "") + ">Start</button>";
        rows += '<span class="k"><i class="dot" style="--st:' + (DOT[evse.status] || "var(--muted)") + '"></i>' + esc(chargerName(evse.uid)) + "</span>" + right;
      });
    });
    setHTML($("nearby"), rows || '<span class="empty">No chargers known yet.</span>');

    var receipts = (emsp.cdrs || []).slice().reverse().map(function (cdr) {
      return "<span>" + kwh(cdr.total_energy) + " · " + esc(chargerName(cdr.cdr_location && cdr.cdr_location.evse_uid)) + "</span><span>" + money(cdr.total_cost && cdr.total_cost.excl_vat) + "</span>";
    }).join("");
    setHTML($("receipts"), receipts || '<span class="empty">None yet.</span>');

    var truth = session && cpo.chargers.filter(function (c) { return c.session_id === session.id; })[0];
    setHTML($("diverge"), (truth && truth.active_session
      ? "<b>Charger: " + kwh(truth.active_session.energy_delivered_kwh) + ". Phone: " + kwh(session.kwh) + ".</b>" : ""));
  }

  $("phone-col").addEventListener("click", function (event) {
    var data = event.target.dataset;
    if (data.start) act(call("POST", "/emsp/api/start", {location_id: data.location, evse_uid: data.start}));
    if (data.stop) act(call("POST", "/emsp/api/stop", {session_id: data.stop}));
  });

  // ---------- under the hood ----------
  var lanes = document.querySelectorAll(".lane");
  function highlight(module, direction) {
    lanes.forEach(function (lane) {
      var on = lane.dataset.module === module;
      lane.classList.toggle("on", on);
      if (on && direction) lane.dataset.dir = direction; else lane.removeAttribute("data-dir");
    });
    document.querySelectorAll(".msg").forEach(function (message) {
      message.classList.toggle("match", !direction && message.dataset.module === module);
    });
  }
  lanes.forEach(function (lane) {
    lane.addEventListener("click", function () {
      document.querySelectorAll(".msg[open]").forEach(function (message) { message.open = false; });
      highlight(lane.dataset.module, null);
    });
  });
  function pretty(text) {
    try { return JSON.stringify(JSON.parse(text), null, 2); } catch (error) { return text || ""; }
  }
  function shortPath(entry) {
    var path = entry.url.replace(/^https?:\/\/[^/]+/, "");
    if (entry.direction === "CPO_TO_EMSP" && entry.module === "commands") return "<response_url>";
    return path.replace(/^\/emsp\/ocpi\/2\.2\.1/, "").replace(/^\/ocpi\/cpo\/2\.2\.1/, "");
  }
  function appendMessages(entries) {
    var log = $("log");
    entries.forEach(function (entry) {
      var out = entry.direction === "CPO_TO_EMSP", failed = entry.status_code === 0 || entry.status_code >= 400;
      var message = document.createElement("details");
      message.className = "msg";
      message.dataset.module = entry.module;
      message.dataset.dir = out ? "out" : "in";
      message.innerHTML = '<summary><span class="t">' + clockTime(entry.recorded_at, true) + '</span><span class="dir ' + (out ? "out" : "in") + '">' +
        (out ? "CPO → eMSP" : "eMSP → CPO") + '</span><span class="t' + (failed ? " fail" : "") + '">' + (entry.status_code || "failed") + "</span>" +
        '<span class="http">' + esc(entry.method + " " + shortPath(entry)) + '</span><span class="say">' + esc(entry.summary) + "</span></summary>" +
        "<pre>" + esc((entry.request_body ? "→ " + pretty(entry.request_body) + "\n" : "") + "← " + pretty(entry.response_body)) + "</pre>";
      message.addEventListener("toggle", function () {
        if (!message.open) return;
        document.querySelectorAll(".msg[open]").forEach(function (other) { if (other !== message) other.open = false; });
        highlight(message.dataset.module, message.dataset.dir);
      });
      log.insertBefore(message, log.firstChild);
      while (log.children.length > MAX_LOG) log.removeChild(log.lastChild);
    });
    messageCount += entries.length;
    $("msg-count").textContent = messageCount + (messageCount === 1 ? " message" : " messages") + " so far, newest first";
  }

  // ---------- header, checklist, legend ----------
  function renderHeader() {
    $("clock-now").textContent = clockTime(cpo.clock.now, true);
    $("phone-time").textContent = clockTime(cpo.clock.now, false);
    document.querySelectorAll("[data-speed]").forEach(function (button) {
      button.setAttribute("aria-pressed", String(parseFloat(button.dataset.speed) === cpo.clock.speed));
    });
  }
  function renderSteps() {
    done.plugged = done.plugged || cpo.chargers.some(function (c) { return !!c.vehicle; }) || cpo.sessions.length > 0;
    if (emsp) {
      done.started = done.started || emsp.sessions.length > 0;
      done.billed = done.billed || emsp.cdrs.length > 0;
    }
    var current = !done.plugged ? "plugged" : !done.started ? "started" : !done.billed ? "billed" : null;
    document.querySelectorAll("#steps li").forEach(function (item) {
      var isDone = done[item.dataset.step];
      item.classList.toggle("done", isDone);
      item.classList.toggle("current", item.dataset.step === current);
      item.querySelector(".num").textContent = isDone ? "✓" : {plugged: 1, started: 2, billed: 3}[item.dataset.step];
    });
    $("after-steps").hidden = current !== null;
    pointAt(nextButton(current));
  }
  // The one button that moves the visitor forward. Whatever state they have got the world into,
  // there is always a sensible next click; Charger 1 never fails, so it is preferred.
  function nextButton(current) {
    if (!current || !emsp) return null;
    var session = mySession();
    if (session) return current === "billed" ? document.querySelector("[data-stop]:not([disabled])") : null;
    if (startPending()) return null;
    var waiting = cpo.chargers.filter(function (c) { return c.state === "PREPARING" || c.state === "FINISHING"; })[0];
    if (waiting) return document.querySelector('[data-start="' + waiting.charger_id + '"]:not([disabled])');
    var free = cpo.chargers.filter(function (c) { return c.state === "AVAILABLE"; })[0];
    return free ? document.querySelector('.charger[data-id="' + free.charger_id + '"] [data-action="plug-in"]') : null;
  }
  function pointAt(target) {
    document.querySelectorAll(".next").forEach(function (element) {
      if (element !== target) element.classList.remove("next");
    });
    if (target && !target.classList.contains("next")) target.classList.add("next");
  }
  $("legend").innerHTML = ["AVAILABLE", "PREPARING", "CHARGING", "FINISHING", "FAULTED"].map(function (state) {
    return '<figure><div class="illus">' + draw(state, state !== "AVAILABLE" && state !== "FAULTED") + "</div><figcaption>" + state +
      '<span class="tells">eMSP sees ' + TOLD[state] + "</span></figcaption></figure>";
  }).join("");

  // ---------- polling ----------
  function poll() {
    return Promise.all([
      call("GET", "/api/state?since=" + lastSequence),
      call("GET", "/emsp/api/state").catch(function () { return null; })
    ]).then(function (results) {
      cpo = results[0];
      emsp = results[1];
      // a different world means everything this page remembers (trace position, checklist) is stale
      if (worldID && cpo.world_id !== worldID) { window.location.reload(); return; }
      worldID = cpo.world_id;
      if (cpo.trace.length) {
        lastSequence = cpo.trace[cpo.trace.length - 1].sequence;
        appendMessages(cpo.trace);
      }
      renderHeader();
      renderChargers();
      $("phone-col").hidden = !emsp;
      if (emsp) renderPhone();
      renderSteps();
    }).catch(function (error) { flash("Lost contact with the simulator: " + error.message); });
  }
  poll();
  setInterval(poll, POLL_MS);
})();
