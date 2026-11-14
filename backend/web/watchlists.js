(() => {
  "use strict";

  // Where "request a list" mail goes. Lists are added by hand in
  // controllers/watchlists/seed.go, so this is the only way in for visitors.
  const REQUEST_EMAIL = "nivas27dev@gmail.com";
  const MAX_PER_LIST = 35; // keep in sync with maxItemsPerList in controller.go

  const SVG_NS = "http://www.w3.org/2000/svg";
  const SPARK = { w: 90, h: 34, pad: 3 };
  const POLL_MS = 6000;
  const RETRY_MS = 15000;
  const PICKED_KEY = "watchlist:picked";
  const TREND_ARROW = { Uptrend: ["↗", "up"], Downtrend: ["↘", "down"], Sideways: ["→", "flat"] };

  const $ = (id) => document.getElementById(id);
  let lists = [];
  let pickedId = null;
  let pollTimer = null;
  let gradSeq = 0;

  function el(tag, cls, text) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  // ---------- server ----------

  async function request(method, url, body) {
    const res = await fetch(url, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (res.status === 204 || res.status === 202) return null;
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`);
    return data;
  }

  function setStatus(text, bad) {
    const s = $("status");
    s.textContent = text;
    s.classList.toggle("bad", !!bad);
  }

  async function load() {
    clearTimeout(pollTimer);
    try {
      const data = await request("GET", "/api/lists");
      lists = data.lists || [];
      renderPicker();
      renderPicked();
      showStatus(data);
      if (data.pending > 0 && data.keyConfigured) pollTimer = setTimeout(load, POLL_MS);
    } catch (err) {
      const down = err.message.includes("404");
      setStatus(down ? "Lists are unavailable — is MongoDB running?" : `Could not load: ${err.message}`, true);
      showEmpty(down ? "The server has no lists to show yet." : "Could not reach the server.");
      pollTimer = setTimeout(load, RETRY_MS);
    }
  }

  function showStatus(d) {
    if (!d.keyConfigured) {
      setStatus("Set TWELVE_DATA_API_KEY in backend/.env and restart to see prices.", true);
    } else if (d.pending > 0) {
      setStatus(`Fetching ${d.pending} more (free plan: 8 requests a minute)…`);
    } else {
      setStatus(`Trend for ${d.date} · each symbol is read once a day`);
    }
  }

  // ---------- the dropdown ----------

  // Keeps the chosen list across reloads, and falls back to the first one.
  function currentList() {
    return lists.find((l) => l.id === pickedId) || lists[0] || null;
  }

  function renderPicker() {
    const sel = $("picker");
    const stored = pickedId || localStorage.getItem(PICKED_KEY);
    if (lists.some((l) => l.id === stored)) pickedId = stored;
    else pickedId = lists.length ? lists[0].id : null;

    sel.replaceChildren(...lists.map((l) => {
      const o = el("option", null, `${l.name} (${l.items.length})`);
      o.value = l.id;
      return o;
    }));
    sel.disabled = lists.length < 1;
    if (pickedId) sel.value = pickedId;
    if (!lists.length) sel.append(el("option", null, "No lists yet"));
  }

  $("picker").addEventListener("change", (e) => {
    pickedId = e.target.value;
    localStorage.setItem(PICKED_KEY, pickedId);
    renderPicked();
  });

  // ---------- the one visible list ----------

  function showEmpty(message) {
    $("stats").hidden = true;
    $("listName").textContent = "No list";
    $("listMeta").textContent = "";
    $("notes").replaceChildren();
    $("rows").replaceChildren(el("li", "empty", message));
  }

  function renderPicked() {
    const list = currentList();
    if (!list) {
      showEmpty("No lists yet. Ask for one and it shows up here.");
      return;
    }

    $("listName").textContent = list.name;
    $("listMeta").textContent = `${list.items.length} of ${MAX_PER_LIST} symbols`;

    renderStats(list);
    renderNotes(list);
    renderRows(list);
  }

  function renderStats(list) {
    const priced = list.items.filter((it) => !it.loading && !it.error);
    $("stats").hidden = false;
    $("statCount").textContent = list.items.length;
    $("statUp").textContent = priced.filter((it) => it.changePct >= 0).length;
    $("statDown").textContent = priced.filter((it) => it.changePct < 0).length;
  }

  function renderNotes(list) {
    const notes = $("notes");
    notes.replaceChildren();

    if (list.pending.length) {
      const n = list.pending.length;
      const next = list.pending.slice(0, 3).join(", ") + (n > 3 ? "…" : "");
      notes.append(el("p", "note", `Looking up ${n} ${n === 1 ? "entry" : "entries"}: ${next}`));
    }
    if (list.unresolved.length) {
      notes.append(el("p", "note warn", `Could not find: ${list.unresolved.join(", ")}`));
    }
  }

  function renderRows(list) {
    const rows = $("rows");
    if (!list.items.length) {
      rows.replaceChildren(el("li", "empty", "This list is empty."));
      return;
    }
    rows.replaceChildren(...list.items.map(itemRow));
  }

  // ---------- one row ----------

  function itemRow(it) {
    const li = el("li", "row");
    const down = it.changePct < 0;
    if (it.loading) li.classList.add("loading");
    else if (it.error) li.classList.add("err");
    else if (down) li.classList.add("down");
    li.title = it.error || `${it.name}${it.trend ? " · " + it.trend : ""}`;

    const left = el("div");
    const sym = el("div", "sym", it.symbol);
    const arrow = TREND_ARROW[it.trend];
    if (arrow && !it.error) {
      const a = el("span", `arrow ${arrow[1]}`, arrow[0]);
      a.title = it.trend;
      sym.append(a);
    }
    left.append(sym, el("div", "name", it.name || it.symbol));
    if (it.error) left.append(el("div", "reason", it.error));
    li.append(left);

    if (it.error) return li;

    li.append(it.loading ? el("div") : sparkline(it.spark, down));
    const right = el("div", "quote");
    right.append(
      el("div", "price", it.loading ? "…" : fmtPrice(it.price)),
      el("div", "badge", it.loading ? "—" : fmtPct(it.changePct)),
    );
    li.append(right);
    return li;
  }

  const fmtPrice = (p) => p.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: p < 10 ? 4 : 2,
  });
  const fmtPct = (p) => `${p >= 0 ? "+" : ""}${p.toFixed(2)}%`;

  // Smooth line over a soft gradient; green when up, red when down.
  function sparkline(values, down) {
    const svg = document.createElementNS(SVG_NS, "svg");
    svg.setAttribute("viewBox", `0 0 ${SPARK.w} ${SPARK.h}`);
    svg.setAttribute("class", "spark");
    if (!values || values.length < 2) return svg;

    const min = Math.min(...values);
    const span = Math.max(...values) - min || 1;
    const pts = values.map((v, k) => [
      (k / (values.length - 1)) * SPARK.w,
      SPARK.pad + (1 - (v - min) / span) * (SPARK.h - SPARK.pad * 2),
    ]);

    let d = `M${pts[0][0]},${pts[0][1]}`;
    for (let k = 1; k < pts.length; k++) {
      const mx = (pts[k - 1][0] + pts[k][0]) / 2;
      const my = (pts[k - 1][1] + pts[k][1]) / 2;
      d += ` Q${pts[k - 1][0]},${pts[k - 1][1]} ${mx},${my}`;
    }
    const last = pts[pts.length - 1];
    d += ` T${last[0]},${last[1]}`;

    const color = down ? "var(--down)" : "var(--up)";
    const id = `g${gradSeq++}`;

    const defs = document.createElementNS(SVG_NS, "defs");
    defs.innerHTML =
      `<linearGradient id="${id}" x1="0" y1="0" x2="0" y2="1">` +
      `<stop offset="0" stop-color="${color}" stop-opacity="0.3"/>` +
      `<stop offset="1" stop-color="${color}" stop-opacity="0"/></linearGradient>`;

    const area = document.createElementNS(SVG_NS, "path");
    area.setAttribute("d", `${d} L${SPARK.w},${SPARK.h} L0,${SPARK.h} Z`);
    area.setAttribute("fill", `url(#${id})`);

    const line = document.createElementNS(SVG_NS, "path");
    line.setAttribute("d", d);
    line.setAttribute("fill", "none");
    line.setAttribute("stroke", color);
    line.setAttribute("stroke-width", "1.6");
    line.setAttribute("stroke-linecap", "round");

    svg.append(defs, area, line);
    return svg;
  }

  // ---------- request a list ----------

  function mailtoHref() {
    const subject = encodeURIComponent("Watchlist request");
    const body = encodeURIComponent(
      `List name:\n\nSymbols (up to ${MAX_PER_LIST}, comma separated):\n\n`,
    );
    return `mailto:${REQUEST_EMAIL}?subject=${subject}&body=${body}`;
  }

  for (const id of ["requestLink", "requestLink2"]) {
    const a = $(id);
    if (a) { a.href = mailtoHref(); a.title = `Email ${REQUEST_EMAIL}`; }
  }

  load();
})();
