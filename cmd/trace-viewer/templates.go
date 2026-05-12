package main

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Trace Timeline: {{.SessionID}}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f5f5f5;
            padding: 20px;
        }

        .container {
            max-width: 1200px;
            margin: 0 auto;
        }

        header {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 30px;
        }

        h1 {
            color: #2c3e50;
            margin-bottom: 10px;
        }

        .metadata {
            font-size: 14px;
            color: #666;
        }

        .metadata span {
            margin-right: 20px;
        }

        #timeline {
            background: white;
            padding: 40px 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            overflow-x: auto;
        }

        .timeline-container {
            display: flex;
            align-items: flex-start;
            gap: 20px;
            min-width: fit-content;
        }

        .turn-node {
            width: 60px;
            height: 60px;
            border-radius: 50%;
            border: 3px solid #3498db;
            background: white;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            cursor: pointer;
            transition: border-color 0.1s;
        }

        .turn-node:hover {
            border-color: #2980b9;
        }

        .turn-node.expanded {
            border-color: #9b59b6;
            background: #f3e5f5;
        }

        .turn-node.error {
            border-color: #e74c3c;
        }

        .turn-label {
            font-size: 10px;
            color: #666;
        }

        .turn-tokens {
            font-size: 9px;
            color: #888;
        }

        .edge {
            width: 30px;
            height: 2px;
            background: #ddd;
            align-self: center;
        }

        .detail-panel {
            margin-left: 72px;
            margin-top: 12px;
            margin-bottom: 20px;
            background: #f8f9fa;
            border-left: 4px solid #9b59b6;
            border-radius: 6px;
            padding: 16px;
            max-width: 700px;
        }

        .detail-header {
            display: flex;
            justify-content: space-between;
            align-items: start;
            margin-bottom: 10px;
        }

        .detail-title {
            font-weight: bold;
            color: #9b59b6;
            font-size: 12px;
        }

        .detail-close {
            font-size: 18px;
            color: #ccc;
            cursor: pointer;
            line-height: 1;
        }

        .detail-close:hover {
            color: #999;
        }

        .detail-metrics {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 10px;
            margin-bottom: 12px;
            font-size: 11px;
            color: #666;
        }

        .tool-list {
            border-top: 1px solid #ddd;
            padding-top: 10px;
            margin-top: 10px;
        }

        .tool-list-title {
            font-weight: 600;
            font-size: 10px;
            color: #888;
            margin-bottom: 6px;
        }

        .tool-item {
            background: white;
            border-radius: 4px;
            padding: 8px;
            font-size: 10px;
            display: flex;
            justify-content: space-between;
            margin-bottom: 4px;
        }

        .hidden {
            display: none;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>📊 Trace Timeline</h1>
            <div class="metadata">
                <span><strong>Session:</strong> {{.SessionID}}</span>
                <span><strong>Query:</strong> {{.Query}}</span>
                <span><strong>Duration:</strong> {{.DurationMs}}ms</span>
                <span><strong>Status:</strong> {{.Status}}</span>
            </div>
        </header>

        <div id="timeline">
            <!-- Timeline rendered by JavaScript -->
        </div>
    </div>

    <script type="application/json" id="trace-data">
{{.TraceJSON}}
    </script>

    <script>
        // Load trace data
        const trace = JSON.parse(document.getElementById('trace-data').textContent);

        // State management
        const state = {
            expandedTurns: new Set()
        };

        // Initialize timeline
        renderTimeline();

        function renderTimeline() {
            const container = document.getElementById('timeline');
            container.innerHTML = '';

            const timelineDiv = document.createElement('div');
            timelineDiv.className = 'timeline-container';

            trace.turns.forEach((turn, index) => {
                // Render turn node
                const node = renderTurnNode(turn, index);
                timelineDiv.appendChild(node);

                // Add edge if not last
                if (index < trace.turns.length - 1) {
                    const edge = document.createElement('div');
                    edge.className = 'edge';
                    timelineDiv.appendChild(edge);
                }
            });

            container.appendChild(timelineDiv);

            // Render detail panels for expanded turns
            state.expandedTurns.forEach(turnIndex => {
                renderDetailPanel(trace.turns[turnIndex], turnIndex);
            });
        }

        function renderTurnNode(turn, index) {
            const node = document.createElement('div');
            node.className = 'turn-node';
            node.id = 'turn-' + index;

            if (state.expandedTurns.has(index)) {
                node.classList.add('expanded');
            }

            const label = document.createElement('div');
            label.className = 'turn-label';
            label.textContent = 'Turn ' + index;

            const tokens = document.createElement('div');
            tokens.className = 'turn-tokens';
            tokens.textContent = formatTokens(turn.tokens_remaining);

            node.appendChild(label);
            node.appendChild(tokens);

            node.onclick = () => toggleTurn(index);

            return node;
        }

        function renderDetailPanel(turn, index) {
            const timeline = document.getElementById('timeline');
            const nodeElement = document.getElementById('turn-' + index);

            // Remove existing panel if present
            const existingPanel = document.getElementById('detail-' + index);
            if (existingPanel) {
                existingPanel.remove();
            }

            const panel = document.createElement('div');
            panel.className = 'detail-panel';
            panel.id = 'detail-' + index;

            panel.innerHTML = ` + "`" + `
                <div class="detail-header">
                    <div class="detail-title">Turn ${index} Details</div>
                    <div class="detail-close" onclick="toggleTurn(${index})">×</div>
                </div>
                <div class="detail-metrics">
                    <div><strong>Duration:</strong> ${turn.duration_ms}ms</div>
                    <div><strong>Tokens:</strong> ${turn.llm_call ? turn.llm_call.input_tokens + ' → ' + turn.llm_call.output_tokens : 'N/A'}</div>
                    <div><strong>Remaining:</strong> ${formatTokens(turn.tokens_remaining)}</div>
                </div>
                <div class="tool-list">
                    <div class="tool-list-title">Tool Calls (${(turn.tool_calls || []).length}):</div>
                    ${(turn.tool_calls || []).map(tc => ` + "`" + `
                        <div class="tool-item">
                            <span><strong>${tc.name}</strong></span>
                            <span>${tc.duration_ms}ms</span>
                        </div>
                    ` + "`" + `).join('')}
                </div>
            ` + "`" + `;

            // Insert after the turn node's parent container
            timeline.appendChild(panel);
        }

        function toggleTurn(index) {
            if (state.expandedTurns.has(index)) {
                state.expandedTurns.delete(index);
            } else {
                state.expandedTurns.add(index);
            }
            renderTimeline();
        }

        function formatTokens(tokens) {
            if (tokens >= 1000) {
                return (tokens / 1000).toFixed(1) + 'k';
            }
            return tokens.toString();
        }
    </script>
</body>
</html>`
