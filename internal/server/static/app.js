(function () {
  "use strict";

  const REFRESH_INTERVAL = 30000; // 30 seconds

  async function fetchJSON(url) {
    const resp = await fetch(url);
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    return resp.json();
  }

  function formatPoints(n) {
    if (n >= 1000000) return (n / 1000000).toFixed(1) + "M";
    if (n >= 1000) return (n / 1000).toFixed(1) + "K";
    return n.toLocaleString();
  }

  function renderStreamers(streamers) {
    const grid = document.getElementById("streamers-grid");
    if (!streamers || streamers.length === 0) {
      grid.innerHTML = '<div class="loading">No streamers tracked yet.</div>';
      return;
    }

    // Sort: online first, then by points descending.
    streamers.sort(function (a, b) {
      if (a.is_online !== b.is_online) return b.is_online ? 1 : -1;
      return b.channel_points - a.channel_points;
    });

    grid.innerHTML = streamers
      .map(function (s) {
        var statusClass = s.is_online ? "online" : "offline";
        var statusText = s.is_online ? "Online" : "Offline";
        var categoryBadge = s.is_category_watched ? '<span class="badge category">CAT</span>' : "";
        var gameText = s.game ? s.game : "";
        var viewersText = s.is_online ? s.viewers_count + " viewers" : "";
        var details = [gameText, viewersText].filter(Boolean).join(" · ");

        return '<div class="streamer-card ' + statusClass + '">' + '  <div class="name"><a href="' + s.streamer_url + '" target="_blank">' + (s.display_name || s.username) + "</a></div>" + '  <div class="status">' + '    <span class="badge ' + statusClass + '">' + statusText + "</span>" + categoryBadge + "  </div>" + '  <div class="details">' + '    <span class="points">' + formatPoints(s.channel_points) + " pts</span>" + (details ? " · " + details : "") + (s.title ? "<br><em>" + escapeHTML(s.title) + "</em>" : "") + "  </div>" + "</div>";
      })
      .join("");
  }

  function renderStats(stats) {
    document.getElementById("total-streamers").textContent = "Streamers: " + stats.total_streamers;
    document.getElementById("online-streamers").textContent = "Online: " + stats.online_streamers;
    document.getElementById("total-points").textContent = "Total Points: " + formatPoints(stats.total_points);

    var tbody = document.getElementById("history-body");
    if (!stats.history || Object.keys(stats.history).length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" style="text-align:center;color:#adadb8">No history yet</td></tr>';
      return;
    }

    var rows = Object.keys(stats.history).map(function (reason) {
      var entry = stats.history[reason];
      return { reason: reason, counter: entry.counter, amount: entry.amount };
    });

    // Sort by amount descending.
    rows.sort(function (a, b) {
      return b.amount - a.amount;
    });

    tbody.innerHTML = rows
      .map(function (r) {
        return "<tr>" + "<td>" + escapeHTML(r.reason) + "</td>" + "<td>" + r.counter + "</td>" + "<td>" + formatPoints(r.amount) + "</td>" + "</tr>";
      })
      .join("");
  }

  function escapeHTML(str) {
    var div = document.createElement("div");
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
  }

  async function refresh() {
    try {
      var results = await Promise.all([fetchJSON("/api/streamers"), fetchJSON("/api/stats")]);
      renderStreamers(results[0]);
      renderStats(results[1]);
    } catch (err) {
      console.error("Dashboard refresh error:", err);
    }
  }

  // Initial load.
  refresh();

  // Auto-refresh.
  setInterval(refresh, REFRESH_INTERVAL);
})();
