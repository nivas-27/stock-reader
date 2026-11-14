(() => {
  "use strict";

  const RANGE_DAYS = { "1M": 30, "3M": 91, "6M": 182, "1Y": 365, "2Y": 730, "3Y": 1095 };
  const SMA_PERIOD = 20;
  const ZONE_EDGE = 0.15; // bottom/top zone = lowest/highest 15% of the period's close range
  const TILE_ASPECT = 1.55; // preferred tile width / height when choosing the grid

  const $ = (id) => document.getElementById(id);
  const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const C = {};

  const state = { page: 1, per: 20, range: "6M", sort: "list", totalPages: 0, totalStocks: 0 };
  const cache = new Map(); // key -> Promise<response>
  let current = []; // analysed items shown on the wall
  let requestSeq = 0;

  // ---------- data ----------

  function ymd(d) {
    const p = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
  }

  function dateWindow(range) {
    const to = new Date();
    const from = new Date(to);
    from.setDate(from.getDate() - RANGE_DAYS[range]);
    return { from: ymd(from), to: ymd(to) };
  }

  function screenKey(page) {
    const w = dateWindow(state.range);
    return `${page}|${state.per}|${w.from}|${w.to}`;
  }

  function fetchScreen(page) {
    const key = screenKey(page);
    if (cache.has(key)) return cache.get(key);

    const w = dateWindow(state.range);
    const qs = new URLSearchParams({ page, limit: state.per, from: w.from, to: w.to });
    const p = fetch(`/kite/history?${qs}`)
      .then(async (res) => {
        const body = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
        return body;
      })
      .catch((err) => {
        cache.delete(key);
        throw err;
      });
    cache.set(key, p);
    return p;
  }

  // Kite candle: [timestamp, open, high, low, close, volume]
  function analyse(item, index) {
    const pts = (item.candles || []).map((k) => ({
      d: String(k[0]).slice(0, 10), o: +k[1], h: +k[2], l: +k[3], c: +k[4], v: +k[5] || 0,
    })).filter((p) => Number.isFinite(p.c));

    const a = { ...item, index, pts, ok: pts.length > 1 && !item.error, error: shortError(item.error) };
    if (!a.ok) return a;

    const closes = pts.map((p) => p.c);
    a.first = closes[0];
    a.last = closes[closes.length - 1];
    a.change = (a.last / a.first - 1) * 100;
    a.hiIdx = closes.indexOf(Math.max(...closes));
    a.loIdx = closes.indexOf(Math.min(...closes));
    a.hi = closes[a.hiIdx];
    a.lo = closes[a.loIdx];
    a.pos = a.hi === a.lo ? 0.5 : (a.last - a.lo) / (a.hi - a.lo);
    a.offHigh = (a.last / a.hi - 1) * 100;
    a.sma = sma(closes, SMA_PERIOD);
    a.trend = trendOf(a);
    a.zone = a.pos <= ZONE_EDGE ? "bottom" : a.pos >= 1 - ZONE_EDGE ? "top" : "";
    return a;
  }

  // "kite api returned status 403: {json}" -> the JSON's message, so tiles stay readable.
  function shortError(err) {
    if (!err) return err;
    const m = /status (\d+): (\{.*\})/.exec(err);
    if (!m) return err;
    try { return `${JSON.parse(m[2]).message} (${m[1]})`; } catch { return err; }
  }

  function sma(values, period) {
    const out = new Array(values.length).fill(null);
    let sum = 0;
    values.forEach((v, i) => {
      sum += v;
      if (i >= period) sum -= values[i - period];
      if (i >= period - 1) out[i] = sum / period;
    });
    return out;
  }

  // Up when price is above a rising 20-day average, down when below a falling one.
  function trendOf(a) {
    const n = a.sma.length - 1;
    const now = a.sma[n];
    const before = a.sma[n - 5];
    if (now == null || before == null) return "Sideways";
    const slope = (now / before - 1) * 100;
    if (a.last > now && slope > 0.5) return "Uptrend";
    if (a.last < now && slope < -0.5) return "Downtrend";
    return "Sideways";
  }

  function sorted(items) {
    const list = items.slice();
    const val = (fn, bad) => (a) => (a.ok ? fn(a) : bad);
    const by = {
      bottom: val((a) => a.pos, Infinity),
      top: val((a) => -a.pos, Infinity),
      gain: val((a) => -a.change, Infinity),
      loss: val((a) => a.change, Infinity),
    }[state.sort];
    if (by) list.sort((x, y) => by(x) - by(y));
    return list;
  }

  // ---------- formatting ----------

  const fmtPrice = (v) => v.toLocaleString("en-IN", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const fmtPct = (v) => `${v >= 0 ? "+" : ""}${v.toFixed(1)}%`;
  const fmtVol = (v) => (v >= 1e7 ? `${(v / 1e7).toFixed(2)} Cr` : v >= 1e5 ? `${(v / 1e5).toFixed(2)} L` : v.toLocaleString("en-IN"));
  const fmtDate = (d) => new Date(`${d}T00:00:00`).toLocaleDateString("en-IN", { day: "2-digit", month: "short", year: "2-digit" });

  function changeHtml(a) {
    const up = a.change >= 0;
    return `<span class="chg"><span class="chg-arrow ${up ? "up" : "down"}">${up ? "▲" : "▼"}</span> ${fmtPct(a.change)}</span>`;
  }

  function escapeHtml(s) {
    return String(s ?? "").replace(/[&<>"']/g, (ch) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
  }

  // ---------- layout ----------

  // Pick the column count whose tiles come closest to TILE_ASPECT while filling the grid area.
  function applyLayout(count) {
    const grid = $("grid");
    if (matchMedia("(max-width: 700px)").matches) return;
    const w = grid.clientWidth - 32;
    const h = grid.clientHeight - 16;
    let best = { cols: 1, score: Infinity };
    for (let cols = 1; cols <= count; cols++) {
      const rows = Math.ceil(count / cols);
      const aspect = (w / cols) / (h / rows);
      const score = Math.abs(Math.log(aspect / TILE_ASPECT)) + (cols * rows - count) * 0.02;
      if (score < best.score) best = { cols, score };
    }
    const rows = Math.ceil(count / best.cols);
    grid.style.gridTemplateColumns = `repeat(${best.cols}, minmax(0, 1fr))`;
    grid.style.gridTemplateRows = `repeat(${rows}, minmax(0, 1fr))`;
  }

  // ---------- canvas helpers ----------

  function prepCanvas(canvas) {
    const dpr = window.devicePixelRatio || 1;
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;
    if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(h * dpr)) {
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
    }
    const ctx = canvas.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    return { ctx, w, h };
  }

  function dashedH(ctx, x0, x1, y) {
    ctx.save();
    ctx.strokeStyle = C.gridLine;
    ctx.lineWidth = 1;
    ctx.setLineDash([3, 3]);
    ctx.beginPath();
    ctx.moveTo(x0, Math.round(y) + 0.5);
    ctx.lineTo(x1, Math.round(y) + 0.5);
    ctx.stroke();
    ctx.restore();
  }

  function dot(ctx, x, y, r, fill) {
    ctx.beginPath();
    ctx.arc(x, y, r + 1.5, 0, Math.PI * 2);
    ctx.fillStyle = C.surface1;
    ctx.fill();
    ctx.beginPath();
    ctx.arc(x, y, r, 0, Math.PI * 2);
    ctx.fillStyle = fill;
    ctx.fill();
  }

  // ---------- tile sparkline ----------

  function drawTile(canvas, a, hoverIdx) {
    const { ctx, w, h } = prepCanvas(canvas);
    if (w < 20 || h < 20) return null;

    const pad = { l: 2, r: 4, t: 12, b: 12 };
    const n = a.pts.length;
    const span = a.hi - a.lo || 1;
    const x = (i) => pad.l + (i / (n - 1)) * (w - pad.l - pad.r);
    const y = (v) => pad.t + (1 - (v - a.lo) / span) * (h - pad.t - pad.b);

    dashedH(ctx, 0, w, y(a.hi));
    dashedH(ctx, 0, w, y(a.lo));

    // area
    const grad = ctx.createLinearGradient(0, pad.t, 0, h - pad.b);
    grad.addColorStop(0, "rgba(57,135,229,0.28)");
    grad.addColorStop(1, "rgba(57,135,229,0)");
    ctx.beginPath();
    a.pts.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.c)) : ctx.moveTo(x(i), y(p.c))));
    ctx.lineTo(x(n - 1), h - pad.b);
    ctx.lineTo(x(0), h - pad.b);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    // 20-day average
    ctx.save();
    ctx.setLineDash([3, 3]);
    ctx.strokeStyle = C.sma;
    ctx.globalAlpha = 0.7;
    ctx.lineWidth = 1;
    ctx.beginPath();
    let started = false;
    a.sma.forEach((v, i) => {
      if (v == null) return;
      started ? ctx.lineTo(x(i), y(v)) : ctx.moveTo(x(i), y(v));
      started = true;
    });
    ctx.stroke();
    ctx.restore();

    // close line
    ctx.strokeStyle = C.series1;
    ctx.lineWidth = 2;
    ctx.lineJoin = "round";
    ctx.beginPath();
    a.pts.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.c)) : ctx.moveTo(x(i), y(p.c))));
    ctx.stroke();

    // period high / low labels
    ctx.font = "10px system-ui, sans-serif";
    ctx.fillStyle = C.textMuted;
    const label = (i, v, above) => {
      const text = `${above ? "H" : "L"} ${fmtPrice(v)}`;
      const tw = ctx.measureText(text).width;
      const tx = Math.min(Math.max(x(i) - tw / 2, 0), w - tw);
      ctx.textBaseline = above ? "bottom" : "top";
      ctx.fillText(text, tx, above ? y(v) - 2 : y(v) + 2);
    };
    label(a.hiIdx, a.hi, true);
    label(a.loIdx, a.lo, false);

    dot(ctx, x(n - 1), y(a.last), 3, C.series1);

    if (hoverIdx != null) {
      ctx.strokeStyle = C.textMuted;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(Math.round(x(hoverIdx)) + 0.5, 0);
      ctx.lineTo(Math.round(x(hoverIdx)) + 0.5, h);
      ctx.stroke();
      dot(ctx, x(hoverIdx), y(a.pts[hoverIdx].c), 4, C.series1);
    }
    return { x, n, pad, w };
  }

  // ---------- wall ----------

  function tileHtml(a) {
    const idx = `<span class="tile-idx">${a.index}</span>`;
    const name = `<div class="tile-name" title="${escapeHtml(a.name)}">${idx}${escapeHtml(a.name)}</div>`;
    if (!a.ok) {
      return `<div class="tile-head">${name}</div>
        <div class="tile-sub"><span class="tile-cat">${escapeHtml(a.category)}</span></div>
        <div class="tile-error">${escapeHtml(a.error || "No candles in this range")}</div>`;
    }
    const tag = a.zone === "bottom" ? `<span class="tag bottom">Near bottom</span>`
      : a.zone === "top" ? `<span class="tag top">Near top</span>`
      : `<span class="tag">${a.trend}</span>`;
    return `<div class="tile-head">${name}<span class="tile-price">₹${fmtPrice(a.last)}</span></div>
      <div class="tile-sub"><span class="tile-cat">${escapeHtml(a.category)}</span>${changeHtml(a)}</div>
      <canvas></canvas>
      <div class="tile-foot">
        <span class="meter" title="Where the last close sits between the period's lowest and highest close"><i style="left:${(a.pos * 100).toFixed(1)}%"></i></span>
        <span class="foot-pos">${Math.round(a.pos * 100)}%</span>
        ${tag}
        <span title="Distance from period high">${a.offHigh.toFixed(1)}%</span>
      </div>`;
  }

  function renderWall() {
    const grid = $("grid");
    const items = sorted(current);
    applyLayout(Math.max(items.length, 1));
    grid.innerHTML = "";

    items.forEach((a) => {
      const tile = document.createElement("div");
      tile.className = `tile${a.ok && a.zone ? ` zone-${a.zone}` : ""}`;
      tile.innerHTML = tileHtml(a);
      grid.appendChild(tile);
      if (!a.ok) return;

      const canvas = tile.querySelector("canvas");
      const tip = document.createElement("div");
      tip.className = "tip";
      tip.hidden = true;
      tile.appendChild(tip);

      tile._draw = (hover) => (tile._geo = drawTile(canvas, a, hover));
      canvas.addEventListener("mousemove", (e) => {
        const g = tile._geo;
        if (!g) return;
        const r = canvas.getBoundingClientRect();
        const i = Math.round(((e.clientX - r.left - g.pad.l) / (g.w - g.pad.l - g.pad.r)) * (g.n - 1));
        const idx = Math.max(0, Math.min(g.n - 1, i));
        tile._draw(idx);
        const p = a.pts[idx];
        tip.textContent = `${fmtDate(p.d)} · ₹${fmtPrice(p.c)}`;
        tip.hidden = false;
        const left = canvas.offsetLeft + g.x(idx) + 8;
        tip.style.left = `${Math.min(left, tile.clientWidth - tip.offsetWidth - 4)}px`;
        tip.style.top = `${canvas.offsetTop}px`;
      });
      canvas.addEventListener("mouseleave", () => {
        tip.hidden = true;
        tile._draw(null);
      });
      tile.addEventListener("click", () => openDetail(a));
    });
    redrawTiles();
  }

  function redrawTiles() {
    document.querySelectorAll(".tile").forEach((t) => t._draw && t._draw(null));
  }

  function renderSkeleton() {
    const grid = $("grid");
    applyLayout(state.per);
    grid.innerHTML = Array.from({ length: state.per }, (_, i) =>
      `<div class="tile loading"><div class="tile-head"><div class="tile-name"><span class="tile-idx">${(state.page - 1) * state.per + i + 1}</span>…</div></div></div>`
    ).join("");
  }

  function setStatus(text, isError) {
    const el = $("status");
    el.hidden = !text;
    el.textContent = text || "";
    el.classList.toggle("error", !!isError);
  }

  function updatePager() {
    const tp = state.totalPages;
    const start = (state.page - 1) * state.per + 1;
    const end = Math.min(state.page * state.per, state.totalStocks || state.page * state.per);
    $("pageInfo").textContent = tp
      ? `${start}–${end} of ${state.totalStocks} · screen ${state.page}/${tp}`
      : `screen ${state.page}`;
    $("prev").disabled = state.page <= 1;
    $("next").disabled = tp > 0 && state.page >= tp;
  }

  async function load() {
    const seq = ++requestSeq;
    writeHash();
    updatePager();
    renderSkeleton();

    const started = Date.now();
    const timer = setInterval(() => {
      setStatus(`Fetching ${state.per} stocks from Kite… ${Math.round((Date.now() - started) / 1000)}s`);
    }, 500);
    setStatus(`Fetching ${state.per} stocks from Kite…`);

    try {
      const res = await fetchScreen(state.page);
      if (seq !== requestSeq) return;
      state.totalPages = res.totalPages;
      state.totalStocks = res.totalStocks;
      if (state.totalPages && state.page > state.totalPages) {
        state.page = state.totalPages;
        return load();
      }
      const offset = (res.page - 1) * res.limit;
      current = (res.data || []).map((item, i) => analyse(item, offset + i + 1));
      updatePager();
      renderWall();
      const errors = new Set(current.map((a) => a.error));
      const allFailed = current.length && current.every((a) => a.error);
      setStatus(allFailed && errors.size === 1 ? `Every stock failed: ${[...errors][0]}` : "", allFailed);
      prefetchNext();
    } catch (err) {
      if (seq !== requestSeq) return;
      $("grid").innerHTML = "";
      setStatus(`Could not load stocks: ${err.message}`, true);
    } finally {
      clearInterval(timer);
    }
  }

  // Warm the next screen so paging forward feels instant.
  function prefetchNext() {
    if (state.totalPages && state.page < state.totalPages) fetchScreen(state.page + 1).catch(() => {});
  }

  // ---------- detail view (candles + volume) ----------

  let detailItem = null;

  function openDetail(a) {
    detailItem = a;
    $("detailTitle").textContent = a.name;
    $("detailSub").innerHTML = `${escapeHtml(a.category)} · ${a.pts.length} sessions · ` +
      `${fmtDate(a.pts[0].d)} → ${fmtDate(a.pts[a.pts.length - 1].d)} · ${changeHtml(a)} · ` +
      `${Math.round(a.pos * 100)}% of range · ${a.trend}`;
    $("detail").hidden = false;
    readout(a.pts.length - 1);
    drawDetail(null);
  }

  function closeDetail() {
    $("detail").hidden = true;
    detailItem = null;
  }

  function readout(i) {
    const p = detailItem.pts[i];
    const s = detailItem.sma[i];
    $("detailReadout").textContent =
      `${fmtDate(p.d)}   O ${fmtPrice(p.o)}  H ${fmtPrice(p.h)}  L ${fmtPrice(p.l)}  C ${fmtPrice(p.c)}\n` +
      `Vol ${fmtVol(p.v)}${s != null ? `   20d avg ${fmtPrice(s)}` : ""}`;
  }

  function niceTicks(min, max, count) {
    const raw = (max - min) / count;
    const mag = Math.pow(10, Math.floor(Math.log10(raw)));
    const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= raw) || raw;
    const ticks = [];
    for (let v = Math.ceil(min / step) * step; v <= max; v += step) ticks.push(v);
    return ticks;
  }

  function drawDetail(hoverIdx) {
    const a = detailItem;
    if (!a) return;
    const canvas = $("detailCanvas");
    const { ctx, w, h } = prepCanvas(canvas);
    const pad = { l: 8, r: 72, t: 8, b: 24 };
    const volH = Math.round((h - pad.t - pad.b) * 0.18);
    const priceB = h - pad.b - volH - 10;
    const n = a.pts.length;
    const lo = Math.min(...a.pts.map((p) => p.l));
    const hi = Math.max(...a.pts.map((p) => p.h));
    const span = hi - lo || 1;
    const maxV = Math.max(...a.pts.map((p) => p.v)) || 1;
    const slot = (w - pad.l - pad.r) / n;
    const x = (i) => pad.l + slot * (i + 0.5);
    const y = (v) => pad.t + (1 - (v - lo) / span) * (priceB - pad.t);

    ctx.font = "11px system-ui, sans-serif";
    ctx.textBaseline = "middle";
    niceTicks(lo, hi, 6).forEach((v) => {
      dashedH(ctx, pad.l, w - pad.r, y(v));
      ctx.fillStyle = C.textMuted;
      ctx.fillText(fmtPrice(v), w - pad.r + 6, y(v));
    });

    // x labels
    ctx.textBaseline = "top";
    const every = Math.max(1, Math.ceil(n / Math.max(2, Math.floor((w - pad.r) / 110))));
    for (let i = 0; i < n; i += every) {
      const t = fmtDate(a.pts[i].d);
      ctx.fillText(t, Math.min(x(i) - ctx.measureText(t).width / 2, w - pad.r - ctx.measureText(t).width), h - pad.b + 6);
    }

    const bodyW = Math.max(1, Math.min(12, slot * 0.7));
    a.pts.forEach((p, i) => {
      const color = p.c >= p.o ? C.good : C.critical;
      const cx = Math.round(x(i)) + 0.5;
      ctx.strokeStyle = color;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(cx, y(p.h));
      ctx.lineTo(cx, y(p.l));
      ctx.stroke();
      const top = y(Math.max(p.o, p.c));
      ctx.fillStyle = color;
      ctx.fillRect(x(i) - bodyW / 2, top, bodyW, Math.max(1, y(Math.min(p.o, p.c)) - top));

      const vh = (p.v / maxV) * volH;
      ctx.globalAlpha = 0.45;
      ctx.fillRect(x(i) - bodyW / 2, h - pad.b - vh, bodyW, vh);
      ctx.globalAlpha = 1;
    });

    ctx.save();
    ctx.strokeStyle = C.sma;
    ctx.setLineDash([4, 3]);
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    let started = false;
    a.sma.forEach((v, i) => {
      if (v == null) return;
      started ? ctx.lineTo(x(i), y(v)) : ctx.moveTo(x(i), y(v));
      started = true;
    });
    ctx.stroke();
    ctx.restore();

    // last close marker on the axis
    ctx.fillStyle = C.series1;
    ctx.fillRect(w - pad.r + 2, y(a.last) - 9, pad.r - 2, 18);
    ctx.fillStyle = C.textPrimary;
    ctx.textBaseline = "middle";
    ctx.fillText(fmtPrice(a.last), w - pad.r + 6, y(a.last));

    if (hoverIdx != null) {
      ctx.strokeStyle = C.textMuted;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(Math.round(x(hoverIdx)) + 0.5, pad.t);
      ctx.lineTo(Math.round(x(hoverIdx)) + 0.5, h - pad.b);
      ctx.stroke();
    }
    canvas._geo = { pad, slot, n };
  }

  $("detailCanvas").addEventListener("mousemove", (e) => {
    const g = e.currentTarget._geo;
    if (!g || !detailItem) return;
    const r = e.currentTarget.getBoundingClientRect();
    const i = Math.max(0, Math.min(g.n - 1, Math.floor((e.clientX - r.left - g.pad.l) / g.slot)));
    readout(i);
    drawDetail(i);
  });
  $("detailCanvas").addEventListener("mouseleave", () => {
    if (!detailItem) return;
    readout(detailItem.pts.length - 1);
    drawDetail(null);
  });
  $("detailClose").addEventListener("click", closeDetail);
  $("detail").addEventListener("click", (e) => e.target === e.currentTarget && closeDetail());

  // ---------- state & events ----------

  function readHash() {
    const h = new URLSearchParams(location.hash.slice(1));
    const page = parseInt(h.get("page"), 10);
    const per = parseInt(h.get("per"), 10);
    if (page > 0) state.page = page;
    if (per > 0 && per <= 100) state.per = per;
    if (RANGE_DAYS[h.get("range")]) state.range = h.get("range");
    if (h.get("sort")) state.sort = h.get("sort");
  }

  function writeHash() {
    const h = new URLSearchParams({ page: state.page, per: state.per, range: state.range, sort: state.sort });
    history.replaceState(null, "", `#${h}`);
  }

  function syncControls() {
    const per = $("perScreen");
    if (![...per.options].some((o) => +o.value === state.per)) per.add(new Option(state.per, state.per));
    per.value = String(state.per);
    $("range").value = state.range;
    $("sort").value = state.sort;
  }

  function go(page) {
    if (page < 1 || (state.totalPages && page > state.totalPages)) return;
    state.page = page;
    load();
  }

  $("prev").addEventListener("click", () => go(state.page - 1));
  $("next").addEventListener("click", () => go(state.page + 1));
  $("reload").addEventListener("click", () => {
    cache.delete(screenKey(state.page));
    load();
  });
  $("range").addEventListener("change", (e) => {
    state.range = e.target.value;
    load();
  });
  $("perScreen").addEventListener("change", (e) => {
    const firstShown = (state.page - 1) * state.per; // keep the same stocks roughly in view
    state.per = +e.target.value;
    state.page = Math.floor(firstShown / state.per) + 1;
    state.totalPages = 0;
    load();
  });
  $("sort").addEventListener("change", (e) => {
    state.sort = e.target.value;
    writeHash();
    if (current.length) renderWall();
  });

  document.addEventListener("keydown", (e) => {
    if (e.target.tagName === "SELECT") return;
    if (e.key === "Escape" && detailItem) return closeDetail();
    if (detailItem) return;
    if (e.key === "ArrowRight") go(state.page + 1);
    if (e.key === "ArrowLeft") go(state.page - 1);
    if (e.key === "r" || e.key === "R") $("reload").click();
  });

  let resizeFrame = 0;
  new ResizeObserver(() => {
    cancelAnimationFrame(resizeFrame);
    resizeFrame = requestAnimationFrame(() => {
      const count = document.querySelectorAll(".tile").length;
      if (count) applyLayout(count);
      redrawTiles();
      drawDetail(null);
    });
  }).observe(document.body);

  Object.assign(C, {
    surface1: css("--surface-1"), gridLine: css("--grid-line"), textPrimary: css("--text-primary"),
    textMuted: css("--text-muted"), series1: css("--series-1"), sma: css("--sma"),
    good: css("--good"), critical: css("--critical"),
  });

  readHash();
  syncControls();
  load();
})();
