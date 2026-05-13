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

        .event-node {
            width: 80px;
            height: 40px;
            border: 2px dashed #f39c12;
            border-radius: 6px;
            background: #fff9e6;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            cursor: pointer;
            transition: all 0.1s;
            font-size: 9px;
        }

        .event-node:hover {
            background: #fff3cc;
            border-color: #e67e22;
        }

        .event-node.expanded {
            border-color: #d35400;
            background: #ffe6cc;
        }

        .event-node.guardrail {
            border-color: #27ae60;
            background: #e8f8f5;
        }

        .event-node.guardrail:hover {
            background: #d5f4e6;
            border-color: #229954;
        }

        .event-node.guardrail.expanded {
            border-color: #1e8449;
            background: #c9e8d9;
        }

        .event-node-type {
            font-weight: 600;
            color: #7d6608;
            text-transform: uppercase;
            font-size: 8px;
            letter-spacing: 0.5px;
        }

        .event-node.guardrail .event-node-type {
            color: #145a32;
        }

        .event-node-label {
            font-size: 9px;
            color: #666;
            margin-top: 2px;
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

        .event-detail-panel {
            margin-left: 72px;
            margin-top: 12px;
            margin-bottom: 20px;
            background: #fff9e6;
            border-left: 4px solid #f39c12;
            border-radius: 6px;
            padding: 16px;
            max-width: 700px;
        }

        .event-detail-panel.guardrail {
            background: #e8f8f5;
            border-left-color: #27ae60;
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
            expandedTurns: new Set(),
            expandedEvents: new Set()
        };

        // Initialize timeline
        renderTimeline();

        function renderTimeline() {
            const container = document.getElementById('timeline');
            container.innerHTML = '';

            const timelineDiv = document.createElement('div');
            timelineDiv.className = 'timeline-container';

            // Collect and merge turns and events into a single timeline
            const items = collectTimelineItems();

            items.forEach((item, index) => {
                if (item.type === 'turn') {
                    const node = renderTurnNode(item.turn, item.turnIndex);
                    timelineDiv.appendChild(node);
                } else if (item.type === 'event') {
                    const node = renderEventNode(item.event, item.eventIndex, item.turnIndex);
                    timelineDiv.appendChild(node);
                }

                // Add edge if not last
                if (index < items.length - 1) {
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

            // Render detail panels for expanded events
            state.expandedEvents.forEach(eventKey => {
                const event = findEvent(eventKey);
                if (event) {
                    renderEventDetailPanel(event.decision, event.turnIndex, event.eventIndex);
                }
            });
        }

        function collectTimelineItems() {
            const items = [];

            trace.turns.forEach((turn, turnIndex) => {
                // Add turn
                items.push({ type: 'turn', turn: turn, turnIndex: turnIndex });

                // Add turn-level guardrail decisions as events after the turn
                if (turn.guardrail_decisions && turn.guardrail_decisions.length > 0) {
                    turn.guardrail_decisions.forEach((decision, eventIndex) => {
                        items.push({
                            type: 'event',
                            event: decision,
                            turnIndex: turnIndex,
                            eventIndex: eventIndex
                        });
                    });
                }
            });

            // Add trace-level guardrail decisions at the end
            if (trace.guardrail_decisions && trace.guardrail_decisions.length > 0) {
                trace.guardrail_decisions.forEach((decision, eventIndex) => {
                    items.push({
                        type: 'event',
                        event: decision,
                        turnIndex: -1,  // Trace-level event
                        eventIndex: eventIndex
                    });
                });
            }

            return items;
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

        function renderEventNode(decision, eventIndex, turnIndex) {
            const node = document.createElement('div');
            node.className = 'event-node';

            // Add guardrail class for guardrail-related events
            if (decision.tool_name && decision.tool_name !== 'context_compaction') {
                node.classList.add('guardrail');
            }

            const eventKey = turnIndex + '-' + eventIndex;
            node.id = 'event-' + eventKey;

            if (state.expandedEvents.has(eventKey)) {
                node.classList.add('expanded');
            }

            const type = document.createElement('div');
            type.className = 'event-node-type';
            type.textContent = decision.compacted ? 'Compact' : 'Guard';

            const label = document.createElement('div');
            label.className = 'event-node-label';
            label.textContent = decision.tool_name || 'system';

            node.appendChild(type);
            node.appendChild(label);

            node.onclick = () => toggleEvent(eventKey);

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

        function renderEventDetailPanel(decision, turnIndex, eventIndex) {
            const timeline = document.getElementById('timeline');
            const eventKey = turnIndex + '-' + eventIndex;

            // Remove existing panel if present
            const existingPanel = document.getElementById('event-detail-' + eventKey);
            if (existingPanel) {
                existingPanel.remove();
            }

            const panel = document.createElement('div');
            panel.className = 'event-detail-panel';
            panel.id = 'event-detail-' + eventKey;

            // Add guardrail class for styling
            if (decision.tool_name && decision.tool_name !== 'context_compaction') {
                panel.classList.add('guardrail');
            }

            const eventType = decision.compacted ? 'Compaction' : 'Guardrail';
            const location = turnIndex >= 0 ? 'Turn ' + turnIndex : 'Trace-level';

            let detailsHTML = ` + "`" + `
                <div class="detail-header">
                    <div class="detail-title">${eventType} Event (${location})</div>
                    <div class="detail-close" onclick="toggleEvent('${eventKey}')">×</div>
                </div>
                <div class="detail-metrics">
                    <div><strong>Tool:</strong> ${decision.tool_name}</div>
                    <div><strong>Proceed:</strong> ${decision.proceed ? 'Yes' : 'No'}</div>
                    <div><strong>Tokens:</strong> ${decision.estimated_tokens}</div>
                </div>
            ` + "`" + `;

            if (decision.reason) {
                detailsHTML += ` + "`" + `<div style="margin-top: 10px; font-size: 11px; color: #666;"><strong>Reason:</strong> ${decision.reason}</div>` + "`" + `;
            }

            if (decision.compacted && decision.removed_content) {
                detailsHTML += ` + "`" + `
                    <div style="margin-top: 10px; border-top: 1px solid #ddd; padding-top: 10px;">
                        <div style="font-weight: 600; font-size: 10px; color: #888; margin-bottom: 6px;">Removed Content:</div>
                        <div style="font-size: 11px; color: #666;">
                            <div><strong>Tool Results:</strong> ${decision.removed_content.tool_results_count}</div>
                            <div><strong>Messages:</strong> ${decision.removed_content.message_count}</div>
                            <div><strong>Summary Tokens:</strong> ${decision.removed_content.summary_tokens}</div>
                        </div>
                    </div>
                ` + "`" + `;
            }

            if (decision.compacted_turns && decision.compacted_turns.length > 0) {
                detailsHTML += ` + "`" + `
                    <div style="margin-top: 8px; font-size: 11px; color: #666;">
                        <strong>Compacted Turns:</strong> ${decision.compacted_turns.join(', ')}
                    </div>
                ` + "`" + `;
            }

            panel.innerHTML = detailsHTML;

            // Insert after the event node's parent container
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

        function toggleEvent(eventKey) {
            if (state.expandedEvents.has(eventKey)) {
                state.expandedEvents.delete(eventKey);
            } else {
                state.expandedEvents.add(eventKey);
            }
            renderTimeline();
        }

        function findEvent(eventKey) {
            const [turnIndexStr, eventIndexStr] = eventKey.split('-');
            const turnIndex = parseInt(turnIndexStr);
            const eventIndex = parseInt(eventIndexStr);

            if (turnIndex >= 0 && trace.turns[turnIndex] && trace.turns[turnIndex].guardrail_decisions) {
                const decision = trace.turns[turnIndex].guardrail_decisions[eventIndex];
                if (decision) {
                    return { decision, turnIndex, eventIndex };
                }
            }

            if (turnIndex === -1 && trace.guardrail_decisions) {
                const decision = trace.guardrail_decisions[eventIndex];
                if (decision) {
                    return { decision, turnIndex, eventIndex };
                }
            }

            return null;
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
