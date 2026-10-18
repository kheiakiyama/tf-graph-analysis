package exporter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

type htmlNodeData struct {
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	DurationMs       int64   `json:"durationMs"`
	DurationStr      string  `json:"durationStr"`
	EarliestStartMs  int64   `json:"earliestStartMs"`
	EarliestFinishMs int64   `json:"earliestFinishMs"`
	SlackMs          int64   `json:"slackMs"`
	SlackStr         string  `json:"slackStr"`
	IsCritical       bool    `json:"isCritical"`
	IsResource       bool    `json:"isResource"`
	SharePct         float64 `json:"sharePct"`
}

type htmlReportData struct {
	TotalDurationMs  int64          `json:"totalDurationMs"`
	TotalDurationStr string         `json:"totalDurationStr"`
	TotalNodes       int            `json:"totalNodes"`
	CriticalCount    int            `json:"criticalCount"`
	CriticalPath     []htmlNodeData `json:"criticalPath"`
	AllNodes         []htmlNodeData `json:"allNodes"`
}

// ExportHTMLReport writes a standalone, zero-external-dependency interactive HTML report.
func ExportHTMLReport(w io.Writer, res *model.AnalysisResult) error {
	totalMs := res.TotalDuration.Milliseconds()
	if totalMs == 0 && res.TotalDuration > 0 {
		totalMs = 1
	}

	// Prepare critical path data
	critData := make([]htmlNodeData, 0, len(res.CriticalPath))
	for _, n := range res.CriticalPath {
		pct := 0.0
		if res.TotalDuration > 0 {
			pct = (float64(n.Duration) / float64(res.TotalDuration)) * 100
		}
		critData = append(critData, htmlNodeData{
			Name:             n.Name,
			Type:             n.Type,
			DurationMs:       n.Duration.Milliseconds(),
			DurationStr:      n.Duration.String(),
			EarliestStartMs:  n.EarliestStart.Milliseconds(),
			EarliestFinishMs: n.EarliestFinish.Milliseconds(),
			SlackMs:          n.Slack.Milliseconds(),
			SlackStr:         n.Slack.String(),
			IsCritical:       n.IsCritical,
			IsResource:       n.IsResource,
			SharePct:         pct,
		})
	}

	// Prepare all nodes sorted by EarliestStart
	allData := make([]htmlNodeData, 0, len(res.AllNodes))
	for _, n := range res.AllNodes {
		pct := 0.0
		if res.TotalDuration > 0 {
			pct = (float64(n.Duration) / float64(res.TotalDuration)) * 100
		}
		allData = append(allData, htmlNodeData{
			Name:             n.Name,
			Type:             n.Type,
			DurationMs:       n.Duration.Milliseconds(),
			DurationStr:      n.Duration.String(),
			EarliestStartMs:  n.EarliestStart.Milliseconds(),
			EarliestFinishMs: n.EarliestFinish.Milliseconds(),
			SlackMs:          n.Slack.Milliseconds(),
			SlackStr:         n.Slack.String(),
			IsCritical:       n.IsCritical,
			IsResource:       n.IsResource,
			SharePct:         pct,
		})
	}

	// Sort allData by EarliestStart ascending (chronological execution order)
	sort.Slice(allData, func(i, j int) bool {
		if allData[i].EarliestStartMs != allData[j].EarliestStartMs {
			return allData[i].EarliestStartMs < allData[j].EarliestStartMs
		}
		return allData[i].DurationMs > allData[j].DurationMs
	})

	reportData := htmlReportData{
		TotalDurationMs:  totalMs,
		TotalDurationStr: res.TotalDuration.String(),
		TotalNodes:       len(res.AllNodes),
		CriticalCount:    len(res.CriticalPath),
		CriticalPath:     critData,
		AllNodes:         allData,
	}

	dataJSON, err := json.Marshal(reportData)
	if err != nil {
		return fmt.Errorf("failed to serialize report data: %w", err)
	}

	htmlContent := strings.Replace(htmlTemplate, "/*DATA_PLACEHOLDER*/", string(dataJSON), 1)
	_, err = io.WriteString(w, htmlContent)
	return err
}

// ExportHTMLReportFile writes the HTML report to a file.
func ExportHTMLReportFile(path string, res *model.AnalysisResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return ExportHTMLReport(f, res)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Terraform Critical Path Report</title>
<style>
:root {
  --bg-primary: #0f172a;
  --bg-secondary: #1e293b;
  --bg-card: #1e293b;
  --text-primary: #f8fafc;
  --text-secondary: #94a3b8;
  --text-muted: #64748b;
  --accent-critical: #ef4444;
  --accent-critical-bg: rgba(239, 68, 68, 0.15);
  --accent-primary: #3b82f6;
  --border-color: #334155;
  --border-hover: #475569;
}

* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background-color: var(--bg-primary);
  color: var(--text-primary);
  line-height: 1.5;
  padding: 24px;
}

.header {
  margin-bottom: 24px;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 16px;
}
.header h1 {
  font-size: 24px;
  font-weight: 700;
  display: flex;
  align-items: center;
  gap: 10px;
}
.header p {
  color: var(--text-secondary);
  font-size: 14px;
  margin-top: 4px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.metric-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 16px;
}
.metric-card .label {
  font-size: 12px;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.metric-card .value {
  font-size: 24px;
  font-weight: 700;
  margin-top: 4px;
}
.metric-card.critical .value { color: var(--accent-critical); }
.metric-card.primary .value { color: var(--accent-primary); }

.section {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 24px;
}
.section h2 {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 16px;
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Timeline Section */
.timeline-controls {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  flex-wrap: wrap;
  gap: 12px;
}
.timeline-controls label {
  font-size: 13px;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}
.search-input {
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  color: var(--text-primary);
  padding: 6px 12px;
  font-size: 13px;
  width: 250px;
}
.search-input:focus {
  outline: none;
  border-color: var(--accent-primary);
}

.timeline-canvas-wrapper {
  overflow-x: auto;
  overflow-y: auto;
  max-height: 550px;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  position: relative;
}
#timelineCanvas {
  display: block;
}

/* Critical Path Chain */
.chain-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.chain-item {
  display: flex;
  align-items: center;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--border-color);
  border-radius: 6px;
  padding: 10px 14px;
  gap: 16px;
  font-size: 13px;
}
.chain-item:hover {
  border-color: var(--accent-critical);
  background: var(--accent-critical-bg);
}
.chain-step {
  font-weight: 700;
  color: var(--accent-critical);
  min-width: 30px;
}
.chain-name {
  flex-grow: 1;
  font-family: monospace;
  word-break: break-all;
}
.chain-dur {
  font-weight: 600;
  color: var(--text-primary);
  min-width: 80px;
  text-align: right;
}
.chain-share {
  color: var(--text-secondary);
  min-width: 60px;
  text-align: right;
}

/* Table */
.table-wrapper {
  overflow-x: auto;
  max-height: 400px;
  overflow-y: auto;
  border: 1px solid var(--border-color);
  border-radius: 6px;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  text-align: left;
}
th {
  background: #111827;
  padding: 10px 14px;
  color: var(--text-secondary);
  font-weight: 600;
  position: sticky;
  top: 0;
  border-bottom: 1px solid var(--border-color);
}
td {
  padding: 8px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  font-family: monospace;
}
tr:hover {
  background: rgba(255, 255, 255, 0.03);
}
.badge-crit {
  background: var(--accent-critical);
  color: #fff;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
}
.tooltip {
  position: absolute;
  background: #000;
  color: #fff;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12px;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.1s;
  z-index: 100;
  border: 1px solid var(--border-color);
  max-width: 320px;
}
</style>
</head>
<body>

<div class="header">
  <h1>⚡ Terraform Critical Path Report</h1>
  <p>Local Standalone Performance Analysis (Zero External Network Requests)</p>
</div>

<div class="metrics-grid">
  <div class="metric-card critical">
    <div class="label">Total Estimated Duration</div>
    <div class="value" id="valTotalDuration">-</div>
  </div>
  <div class="metric-card critical">
    <div class="label">Critical Path Steps</div>
    <div class="value" id="valCritCount">-</div>
  </div>
  <div class="metric-card primary">
    <div class="label">Total Nodes Analyzed</div>
    <div class="value" id="valTotalNodes">-</div>
  </div>
  <div class="metric-card">
    <div class="label">Bottleneck Concentration</div>
    <div class="value" id="valConcentration">-</div>
  </div>
</div>

<div class="section">
  <h2>⏱️ Interactive Execution Timeline</h2>
  <div class="timeline-controls">
    <div style="display:flex; gap:16px; align-items:center; flex-wrap:wrap;">
      <label>
        <input type="checkbox" id="chkOnlyResources" checked>
        Resources Only
      </label>
      <label>
        <input type="checkbox" id="chkOnlyCritical">
        Show Critical Path Only
      </label>
      <label>
        <input type="checkbox" id="chkHideZero" checked>
        Hide 0s Nodes
      </label>
      <label style="color:var(--text-secondary);">
        Limit:
        <select id="selLimit" style="background:var(--bg-primary); color:var(--text-primary); border:1px solid var(--border-color); border-radius:4px; padding:2px 6px; margin-left:4px;">
          <option value="1000" selected>1000</option>
          <option value="all">All</option>
        </select>
      </label>
    </div>
    <div style="display:flex; align-items:center; gap:8px;">
      <span id="searchCount" style="font-size:12px; color:var(--text-secondary);"></span>
      <input type="text" id="timelineSearch" class="search-input" placeholder="Search resources...">
    </div>
  </div>
  <div class="timeline-canvas-wrapper" id="canvasWrapper">
    <canvas id="timelineCanvas" height="400"></canvas>
  </div>
</div>

<div class="section">
  <h2>🔥 Critical Path Chain (Sequential Bottleneck)</h2>
  <p style="color: var(--text-secondary); font-size: 13px; margin-bottom: 12px;">
    These resources ran sequentially and directly determined the total execution time.
  </p>
  <div class="chain-list" id="chainList"></div>
</div>

<div class="section">
  <h2>📊 Resource Duration Breakdown (Top Slowest)</h2>
  <div class="table-wrapper">
    <table>
      <thead>
        <tr>
          <th>Resource Name</th>
          <th>Duration</th>
          <th>Critical Path?</th>
          <th>Slack (Float)</th>
          <th>Offset (Start)</th>
        </tr>
      </thead>
      <tbody id="tableBody"></tbody>
    </table>
  </div>
</div>

<div class="tooltip" id="tooltip"></div>

<script>
const reportData = /*DATA_PLACEHOLDER*/;

// Initialize metrics
document.getElementById('valTotalDuration').innerText = reportData.totalDurationStr;
document.getElementById('valCritCount').innerText = reportData.criticalCount;
document.getElementById('valTotalNodes').innerText = reportData.totalNodes.toLocaleString();

let critDurMs = 0;
reportData.criticalPath.forEach(n => critDurMs += n.durationMs);
const concentration = reportData.totalDurationMs > 0 ? (critDurMs / reportData.totalDurationMs * 100).toFixed(1) + '%' : '100%';
document.getElementById('valConcentration').innerText = concentration;

// Render Critical Chain
const chainList = document.getElementById('chainList');
reportData.criticalPath.forEach((node, idx) => {
  const item = document.createElement('div');
  item.className = 'chain-item';
  item.innerHTML = ` + "`" + `
    <div class="chain-step">#${idx + 1}</div>
    <div class="chain-name">${escapeHtml(node.name)}</div>
    <div class="chain-dur">${node.durationStr}</div>
    <div class="chain-share">${node.sharePct.toFixed(1)}%</div>
  ` + "`" + `;
  chainList.appendChild(item);
});

// Render Table (Top slowest resources)
const tableBody = document.getElementById('tableBody');
const sortedByDuration = [...reportData.allNodes].sort((a, b) => b.durationMs - a.durationMs);
sortedByDuration.slice(0, 50).forEach(node => {
  const tr = document.createElement('tr');
  const badge = node.isCritical ? '<span class="badge-crit">CRITICAL</span>' : '<span style="color:var(--text-muted)">No</span>';
  tr.innerHTML = ` + "`" + `
    <td>${escapeHtml(node.name)}</td>
    <td><strong>${node.durationStr}</strong></td>
    <td>${badge}</td>
    <td>${node.slackStr}</td>
    <td>${formatTime(node.earliestStartMs)}</td>
  ` + "`" + `;
  tableBody.appendChild(tr);
});

// Format time utility
function formatTime(ms) {
  if (ms < 1000) return ms + 'ms';
  const sec = (ms / 1000).toFixed(1);
  if (ms < 60000) return sec + 's';
  const m = Math.floor(ms / 60000);
  const s = Math.round((ms % 60000) / 1000);
  return m + 'm ' + s + 's';
}

// Canvas Timeline Renderer
const canvas = document.getElementById('timelineCanvas');
const ctx = canvas.getContext('2d');
const wrapper = document.getElementById('canvasWrapper');
const tooltip = document.getElementById('tooltip');

let itemsToRender = [];
let hoveredItem = null;

function updateTimelineData() {
  const onlyResources = document.getElementById('chkOnlyResources').checked;
  const onlyCrit = document.getElementById('chkOnlyCritical').checked;
  const hideZero = document.getElementById('chkHideZero').checked;
  const limitVal = document.getElementById('selLimit').value;
  const query = document.getElementById('timelineSearch').value.toLowerCase().trim();

  // AllNodes is already sorted chronologically by EarliestStart
  let list = onlyCrit ? [...reportData.criticalPath] : [...reportData.allNodes];

  if (onlyResources) {
    list = list.filter(n => n.isResource);
  }

  if (hideZero) {
    list = list.filter(n => n.durationMs > 0);
  }

  const maxSafeItems = 1200; // Browsers crash if canvas height exceeds ~32,000px

  if (query) {
    list = list.filter(n => n.name.toLowerCase().includes(query));
    if (list.length > maxSafeItems) {
      document.getElementById('searchCount').innerText = list.length + ' matches (showing top ' + maxSafeItems + ')';
      itemsToRender = list.slice(0, maxSafeItems);
    } else {
      document.getElementById('searchCount').innerText = list.length + ' matches';
      itemsToRender = list;
    }
  } else {
    if (limitVal === 'all') {
      if (list.length > maxSafeItems) {
        document.getElementById('searchCount').innerText = 'Showing top ' + maxSafeItems + ' of ' + list.length + ' (Canvas limit)';
        itemsToRender = list.slice(0, maxSafeItems);
      } else {
        document.getElementById('searchCount').innerText = list.length + ' items (All)';
        itemsToRender = list;
      }
    } else {
      const num = parseInt(limitVal, 10);
      document.getElementById('searchCount').innerText = Math.min(num, list.length) + ' of ' + list.length + ' items';
      itemsToRender = list.slice(0, num);
    }
  }

  renderTimeline();
}

document.getElementById('chkOnlyResources').addEventListener('change', updateTimelineData);
document.getElementById('chkOnlyCritical').addEventListener('change', updateTimelineData);
document.getElementById('chkHideZero').addEventListener('change', updateTimelineData);
document.getElementById('selLimit').addEventListener('change', updateTimelineData);
document.getElementById('timelineSearch').addEventListener('input', updateTimelineData);

function renderTimeline() {
  const rowHeight = 24;
  const paddingLeft = 300;
  const paddingRight = 40;
  const paddingTop = 30;
  const height = Math.max(300, Math.min(30000, paddingTop + itemsToRender.length * rowHeight + 20));
  const width = Math.max(wrapper.clientWidth, 900);

  canvas.width = width;
  canvas.height = height;

  ctx.clearRect(0, 0, width, height);

  const maxTimeMs = Math.max(1, reportData.totalDurationMs);
  const chartWidth = width - paddingLeft - paddingRight;

  // Draw time grid
  ctx.strokeStyle = '#223049';
  ctx.fillStyle = '#64748b';
  ctx.font = '11px sans-serif';

  const steps = 6;
  for (let i = 0; i <= steps; i++) {
    const x = paddingLeft + (chartWidth / steps) * i;
    const timeVal = Math.round((maxTimeMs / steps) * i);
    ctx.beginPath();
    ctx.moveTo(x, paddingTop - 10);
    ctx.lineTo(x, height - 10);
    ctx.stroke();
    ctx.fillText(formatTime(timeVal), x - 15, paddingTop - 15);
  }

  // Draw items
  itemsToRender.forEach((item, idx) => {
    const y = paddingTop + idx * rowHeight;
    const startX = paddingLeft + (item.earliestStartMs / maxTimeMs) * chartWidth;
    const barWidth = Math.max(4, (item.durationMs / maxTimeMs) * chartWidth);

    // Label
    ctx.fillStyle = item.isCritical ? '#fca5a5' : '#cbd5e1';
    ctx.font = item.isCritical ? 'bold 12px sans-serif' : '11px sans-serif';
    const truncated = truncate(item.name, 38);
    ctx.fillText(truncated, 10, y + 15);

    // Bar
    ctx.fillStyle = item.isCritical ? '#ef4444' : '#3b82f6';
    if (hoveredItem === item) {
      ctx.fillStyle = '#f87171';
    }
    ctx.fillRect(startX, y + 4, barWidth, rowHeight - 8);

    // Store bounding box for tooltip
    item._bbox = { x: startX, y: y + 4, w: barWidth, h: rowHeight - 8 };
  });
}

canvas.addEventListener('mousemove', (e) => {
  const rect = canvas.getBoundingClientRect();
  const mouseX = e.clientX - rect.left;
  const mouseY = e.clientY - rect.top;

  let found = null;
  for (const item of itemsToRender) {
    if (item._bbox &&
        mouseX >= item._bbox.x && mouseX <= item._bbox.x + item._bbox.w &&
        mouseY >= item._bbox.y && mouseY <= item._bbox.y + item._bbox.h) {
      found = item;
      break;
    }
  }

  hoveredItem = found;
  if (found) {
    tooltip.style.opacity = '1';
    tooltip.style.left = (e.pageX + 12) + 'px';
    tooltip.style.top = (e.pageY + 12) + 'px';
    tooltip.innerHTML = ` + "`" + `
      <strong>${escapeHtml(found.name)}</strong><br>
      Start Offset: ${formatTime(found.earliestStartMs)}<br>
      Finish Time: ${formatTime(found.earliestFinishMs)}<br>
      Duration: ${found.durationStr}<br>
      Slack: ${found.slackStr}<br>
      Critical: ${found.isCritical ? '<span style="color:#ef4444;font-weight:bold;">YES (★)</span>' : 'No'}
    ` + "`" + `;
  } else {
    tooltip.style.opacity = '0';
  }
  renderTimeline();
});

canvas.addEventListener('mouseleave', () => {
  tooltip.style.opacity = '0';
  hoveredItem = null;
  renderTimeline();
});

function escapeHtml(str) {
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function truncate(str, max) {
  return str.length > max ? str.substring(0, max - 3) + '...' : str;
}

window.addEventListener('resize', renderTimeline);
updateTimelineData();
</script>
</body>
</html>
`
