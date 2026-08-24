document.addEventListener('DOMContentLoaded', () => {
    // State counters
    let state = {
        positions: {},
        totalEvents: 0,
        acceptedEvents: 0,
        duplicateEvents: 0,
        rejectedEvents: 0,
        logEntries: []
    };

    // DOM Elements
    const statusBadge = document.getElementById('connectionStatus');
    const statusText = document.getElementById('statusText');
    const valActiveSymbols = document.getElementById('valActiveSymbols');
    const valTotalEvents = document.getElementById('valTotalEvents');
    const valAcceptedEvents = document.getElementById('valAcceptedEvents');
    const valDuplicateEvents = document.getElementById('valDuplicateEvents');
    const valRejectedEvents = document.getElementById('valRejectedEvents');
    const logTableBody = document.getElementById('logTableBody');
    const jsonDisplay = document.getElementById('jsonDisplay');
    const logSearchInput = document.getElementById('logSearchInput');
    const btnReset = document.getElementById('btnReset');
    const btnFetchPosition = document.getElementById('btnFetchPosition');
    const csvFileInput = document.getElementById('csvFileInput');
    const uploadDropZone = document.getElementById('uploadDropZone');

    // Chart.js Setup
    const ctx = document.getElementById('positionsChart').getContext('2d');
    const positionsChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: [],
            datasets: [{
                label: 'Net Position',
                data: [],
                backgroundColor: [],
                borderColor: [],
                borderWidth: 1.5,
                borderRadius: 6
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: {
                duration: 400
            },
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        label: function(context) {
                            const val = context.raw;
                            return ` Net Position: ${val > 0 ? '+' : ''}${val}`;
                        }
                    }
                }
            },
            scales: {
                x: {
                    grid: { color: 'rgba(0, 0, 0, 0.05)' },
                    ticks: { color: '#64748b', font: { family: 'Plus Jakarta Sans', size: 11 } }
                },
                y: {
                    grid: { color: 'rgba(0, 0, 0, 0.06)' },
                    ticks: { color: '#64748b', font: { family: 'JetBrains Mono', size: 11 } }
                }
            }
        }
    });

    // 1. Setup Server-Sent Events (SSE) Stream
    function initSSEStream() {
        const evtSource = new EventSource('/events/stream');

        evtSource.onopen = () => {
            statusBadge.className = 'status-badge connected';
            statusText.textContent = 'STREAM LIVE';
        };

        evtSource.onerror = () => {
            statusBadge.className = 'status-badge disconnected';
            statusText.textContent = 'STREAM DISCONNECTED';
        };

        evtSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);

                if (data.type === 'INIT') {
                    state.positions = data.positions || {};
                    if (Array.isArray(data.recent_events) && data.recent_events.length > 0) {
                        state.logEntries = data.recent_events;
                        state.totalEvents = data.recent_events.length;
                        state.acceptedEvents = data.recent_events.filter(e => e.status === 'ACCEPTED').length;
                        state.duplicateEvents = data.recent_events.filter(e => e.status === 'DUPLICATE').length;
                        state.rejectedEvents = data.recent_events.filter(e => e.status === 'REJECTED').length;
                    }
                    updateUI();
                    return;
                }

                if (data.type === 'RESET') {
                    state.positions = {};
                    state.totalEvents = 0;
                    state.acceptedEvents = 0;
                    state.duplicateEvents = 0;
                    state.rejectedEvents = 0;
                    state.logEntries = [];
                    updateUI();
                    return;
                }

                state.totalEvents++;
                if (data.status === 'ACCEPTED') {
                    state.acceptedEvents++;
                    if (data.symbol) {
                        state.positions[data.symbol] = data.net_position;
                    }
                } else if (data.status === 'DUPLICATE') {
                    state.duplicateEvents++;
                } else if (data.status === 'REJECTED') {
                    state.rejectedEvents++;
                }

                // Add to log entries (keep max 200)
                state.logEntries.unshift(data);
                if (state.logEntries.length > 200) {
                    state.logEntries.pop();
                }

                updateUI();
            } catch (e) {
                console.error("Failed to parse SSE event payload:", e);
            }
        };
    }

    // 2. Update UI Components
    function updateUI() {
        // Stats
        const symbols = Object.keys(state.positions);
        valActiveSymbols.textContent = symbols.length;
        valTotalEvents.textContent = state.totalEvents;
        valAcceptedEvents.textContent = state.acceptedEvents;
        valDuplicateEvents.textContent = state.duplicateEvents;
        valRejectedEvents.textContent = state.rejectedEvents;

        // Chart Update
        const labels = symbols;
        const values = symbols.map(s => state.positions[s]);
        const bgColors = values.map(v => v > 0 ? 'rgba(16, 185, 129, 0.7)' : (v < 0 ? 'rgba(244, 63, 94, 0.7)' : 'rgba(148, 163, 184, 0.5)'));
        const borderColors = values.map(v => v > 0 ? '#10b981' : (v < 0 ? '#f43f5e' : '#94a3b8'));

        positionsChart.data.labels = labels;
        positionsChart.data.datasets[0].data = values;
        positionsChart.data.datasets[0].backgroundColor = bgColors;
        positionsChart.data.datasets[0].borderColor = borderColors;
        positionsChart.update('none');

        // JSON Inspector Update
        jsonDisplay.textContent = JSON.stringify(state.positions, null, 2);

        // Audit Log Table Update
        renderLogTable();
    }

    // 3. Render Log Table
    function renderLogTable() {
        const filter = logSearchInput.value.toLowerCase().trim();
        const filtered = state.logEntries.filter(item => {
            if (!filter) return true;
            return (item.symbol && item.symbol.toLowerCase().includes(filter)) ||
                   (item.event_id && item.event_id.toLowerCase().includes(filter)) ||
                   (item.status && item.status.toLowerCase().includes(filter));
        });

        if (filtered.length === 0) {
            logTableBody.innerHTML = `
                <tr class="empty-row">
                    <td colspan="6">No telemetry matching "${filter}"</td>
                </tr>
            `;
            return;
        }

        logTableBody.innerHTML = filtered.slice(0, 50).map(item => {
            const statusClass = item.status ? item.status.toLowerCase() : 'accepted';
            const netPosStr = item.net_position !== undefined ? (item.net_position > 0 ? `+${item.net_position}` : item.net_position) : '-';
            const reasonStr = item.reason || (item.status === 'ACCEPTED' ? `Applied to state (${item.symbol})` : 'OK');
            const timeStr = item.timestamp ? new Date(item.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString();

            return `
                <tr>
                    <td><span class="tag-status ${statusClass}">${item.status || 'OK'}</span></td>
                    <td>${item.event_id || '-'}</td>
                    <td><strong>${item.symbol || '-'}</strong></td>
                    <td class="${item.net_position > 0 ? 'text-green' : (item.net_position < 0 ? 'text-red' : '')}">${netPosStr}</td>
                    <td class="text-muted">${reasonStr}</td>
                    <td class="text-muted">${timeStr}</td>
                </tr>
            `;
        }).join('');
    }

    // 4. Manual API Query
    btnFetchPosition.addEventListener('click', async () => {
        window.open('/position', '_blank');
        try {
            const resp = await fetch('/api/position');
            const data = await resp.json();
            state.positions = data;
            jsonDisplay.textContent = JSON.stringify(data, null, 2);
            updateUI();
        } catch (err) {
            console.error('Failed to query GET /position:', err);
        }
    });

    // 5. Reset Engine State
    if (btnReset) {
        btnReset.addEventListener('click', async () => {
            if (!confirm('Are you sure you want to reset all in-memory net positions and idempotency state?')) return;
            try {
                await fetch('/reset', { method: 'POST' });
                state.positions = {};
                state.totalEvents = 0;
                state.acceptedEvents = 0;
                state.duplicateEvents = 0;
                state.rejectedEvents = 0;
                state.logEntries = [];
                updateUI();
            } catch (err) {
                alert('Failed to reset state: ' + err.message);
            }
        });
    }

    // 6. CSV File Drag & Drop / Upload
    csvFileInput.addEventListener('change', (e) => {
        if (e.target.files.length > 0) {
            uploadCSVFile(e.target.files[0]);
        }
    });

    uploadDropZone.addEventListener('dragover', (e) => {
        e.preventDefault();
        uploadDropZone.style.borderColor = '#38bdf8';
    });

    uploadDropZone.addEventListener('dragleave', () => {
        uploadDropZone.style.borderColor = 'rgba(56, 189, 248, 0.3)';
    });

    uploadDropZone.addEventListener('drop', (e) => {
        e.preventDefault();
        uploadDropZone.style.borderColor = 'rgba(56, 189, 248, 0.3)';
        if (e.dataTransfer.files.length > 0) {
            uploadCSVFile(e.dataTransfer.files[0]);
        }
    });

    async function uploadCSVFile(file) {
        const formData = new FormData();
        formData.append('file', file);

        try {
            statusText.textContent = 'UPLOADING CSV...';
            const resp = await fetch('/events/bulk', {
                method: 'POST',
                body: formData
            });
            const result = await resp.json();
            state.positions = result.positions || state.positions;
            updateUI();
            statusText.textContent = 'STREAM LIVE';
        } catch (err) {
            alert('Failed to process uploaded CSV: ' + err.message);
            statusText.textContent = 'STREAM LIVE';
        }
    }

    logSearchInput.addEventListener('input', renderLogTable);

    // Initialize
    initSSEStream();
    btnFetchPosition.click();
});
