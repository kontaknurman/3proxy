"use strict";

function toast(text) {
  const el = document.createElement("div");
  el.className = "toast";
  el.textContent = text;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 1500);
}

// navigator.clipboard needs HTTPS; plain HTTP falls back to execCommand.
function copy(text) {
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(() => toast("Disalin"));
    return;
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  document.execCommand("copy");
  ta.remove();
  toast("Disalin");
}

function randomString(len) {
  const chars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  const buf = new Uint32Array(len);
  crypto.getRandomValues(buf);
  let s = "";
  for (const v of buf) s += chars[v % chars.length];
  return s;
}

function bytes(n) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return (i === 0 ? Math.round(n) : n.toFixed(1)) + " " + units[i];
}

function duration(secs) {
  secs = Math.floor(secs);
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  if (d > 0) return d + " hari " + h + " jam";
  if (h > 0) return h + " jam " + m + " mnt";
  return m + " mnt " + (secs % 60) + " dtk";
}

function pct(used, total) {
  return total > 0 ? (used / total) * 100 : 0;
}

function set(key, text) {
  document.querySelectorAll('[data-s="' + key + '"]').forEach((el) => {
    el.textContent = text;
  });
}

function setBar(key, value) {
  const el = document.querySelector('[data-bar="' + key + '"]');
  if (!el) return;
  el.style.width = Math.min(100, value).toFixed(1) + "%";
  el.className = value >= 90 ? "crit" : value >= 75 ? "warn" : "";
}

function showStatus(s) {
  set("hostname", s.hostname || "–");
  set("os", s.os);
  set("kernel", s.kernel ? "Kernel " + s.kernel : "");
  set("uptime", duration(s.uptime));
  set("load", s.load.map((v) => v.toFixed(2)).join(" "));
  set("cores", s.cores);
  set("cpu", s.cpu.toFixed(1) + "%");
  setBar("cpu", s.cpu);
  const mem = pct(s.mem_used, s.mem_total);
  set("mem", bytes(s.mem_used) + " / " + bytes(s.mem_total) + " (" + mem.toFixed(0) + "%)");
  setBar("mem", mem);
  set("swap", s.swap_total ? bytes(s.swap_used) + " / " + bytes(s.swap_total) : "tidak ada");
  const disk = pct(s.disk_used, s.disk_total);
  set("disk", bytes(s.disk_used) + " / " + bytes(s.disk_total) + " (" + disk.toFixed(0) + "%)");
  setBar("disk", disk);
  set("rxrate", bytes(s.net_rx_rate) + "/s");
  set("txrate", bytes(s.net_tx_rate) + "/s");
  set("rx", bytes(s.net_rx));
  set("tx", bytes(s.net_tx));

  const proxy = document.querySelector('[data-s="proxy"]');
  if (s.proxy.running) {
    proxy.textContent = "● Berjalan";
    proxy.className = "stat-value up";
    set("proxysub", "PID " + s.proxy.pid + " · " + s.proxy.threads + " thread · RAM " +
      bytes(s.proxy.rss) + " · up " + duration(s.proxy.uptime));
  } else {
    proxy.textContent = "● Mati";
    proxy.className = "stat-value down";
    set("proxysub", "Proses 3proxy tidak ditemukan");
  }

  document.querySelectorAll("[data-conn]").forEach((el) => {
    const n = s.conns[el.dataset.conn];
    el.textContent = n === undefined ? "–" : n;
  });
}

async function refresh() {
  try {
    const res = await fetch("/api/status", { cache: "no-store" });
    if (res.status === 401) {
      location.href = "/login";
      return;
    }
    if (res.ok) showStatus(await res.json());
  } catch (e) {
    // The panel may be restarting; try again on the next tick.
  }
}

document.addEventListener("DOMContentLoaded", () => {
  document.querySelectorAll("[data-copy]").forEach((btn) => {
    btn.addEventListener("click", () => copy(btn.dataset.copy));
  });
  document.querySelectorAll("[data-gen]").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.getElementById(btn.dataset.gen).value = randomString(Number(btn.dataset.len) || 12);
    });
  });
  document.querySelectorAll("form[data-confirm]").forEach((form) => {
    form.addEventListener("submit", (e) => {
      if (!confirm(form.dataset.confirm)) e.preventDefault();
    });
  });
  if (document.getElementById("status")) {
    refresh();
    setInterval(refresh, 3000);
  }
});
