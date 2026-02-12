// Overview Page - Enhanced Dashboard

const OverviewPage = {
    refreshInterval: null,

    async render() {
        const content = document.getElementById('content');

        try {
            // Fetch dashboard data (uses enhanced endpoint)
            const [sessions, children, dashboard] = await Promise.all([
                API.getSessions(),
                API.getChildren(),
                API.get('/api/system/dashboard').catch(() => API.getSystemStatus())
            ]);

            // Calculate stats
            const totalOnline = sessions.length;
            const totalChildren = children.length;
            const lowQuota = dashboard.low_quota_alerts || children.filter(c => c.remaining_min <= 15 && c.remaining_min > 0).length;
            const totalUsedToday = children.reduce((sum, c) => sum + c.used_today_min, 0);

            content.innerHTML = `
                <div class="page-header">
                    <h1>Dashboard</h1>
                    <span style="color: var(--text-secondary); font-size: 0.9rem;">
                        Uptime: ${dashboard.uptime}
                    </span>
                </div>

                <!-- Main Stats -->
                <div class="stats-grid">
                    <div class="stat-card">
                        <div class="stat-value">${totalOnline}</div>
                        <div class="stat-label">Online Now</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value">${totalChildren}</div>
                        <div class="stat-label">Total Children</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value">${formatMinutes(totalUsedToday)}</div>
                        <div class="stat-label">Total Usage Today</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value ${lowQuota > 0 ? 'warning' : ''}">${lowQuota}</div>
                        <div class="stat-label">Low Quota Alerts</div>
                    </div>
                </div>

                <!-- System Resources & Services -->
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-bottom: 1rem;">
                    <div class="card" style="margin-bottom: 0;">
                        <div class="card-header">
                            <h2>System Resources</h2>
                        </div>
                        ${this.renderResourceBars(dashboard)}
                    </div>

                    <div class="card" style="margin-bottom: 0;">
                        <div class="card-header">
                            <h2>Services</h2>
                        </div>
                        <div class="service-status-grid">
                            <div class="service-status">
                                <span class="status-dot ${dashboard.opennds_running ? 'active' : ''}"></span>
                                <div>
                                    <div>OpenNDS</div>
                                    <div style="font-size: 0.8rem; color: var(--text-secondary);">
                                        ${dashboard.opennds_running ? `${dashboard.opennds_clients || 0} clients` : 'Stopped'}
                                    </div>
                                </div>
                            </div>
                            <div class="service-status">
                                <span class="status-dot ${dashboard.dnsmasq_running ? 'active' : ''}"></span>
                                <div>
                                    <div>Dnsmasq</div>
                                    <div style="font-size: 0.8rem; color: var(--text-secondary);">
                                        ${dashboard.dnsmasq_running ? 'Running' : 'Stopped'}
                                    </div>
                                </div>
                            </div>
                        </div>
                        ${dashboard.cpu_load ? `
                            <div style="margin-top: 0.75rem; font-size: 0.85rem; color: var(--text-secondary);">
                                CPU Load: ${dashboard.cpu_load}
                            </div>
                        ` : ''}
                    </div>
                </div>

                <!-- Active Sessions -->
                <div class="card">
                    <div class="card-header">
                        <h2>Active Sessions</h2>
                        <span style="color: var(--text-secondary); font-size: 0.85rem;">
                            Auto-refresh every 30s
                        </span>
                    </div>
                    ${sessions.length === 0 ? `
                        <div class="empty-state">
                            <p>No active sessions</p>
                        </div>
                    ` : `
                        <table>
                            <thead>
                                <tr>
                                    <th>Child</th>
                                    <th>Device</th>
                                    <th>Started</th>
                                    <th>Duration</th>
                                    <th>Remaining</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${sessions.map(s => `
                                    <tr>
                                        <td><strong>${escapeHtml(s.child_name)}</strong></td>
                                        <td><code>${s.mac}</code></td>
                                        <td>${formatTime(s.started_at)}</td>
                                        <td>${formatMinutes(s.duration_min)}</td>
                                        <td>
                                            <span class="${s.remaining_min <= 15 ? 'tag active' : ''}">${formatMinutes(s.remaining_min)}</span>
                                        </td>
                                        <td>
                                            <div class="btn-group">
                                                <button class="btn-small btn-secondary" onclick="OverviewPage.extendSession('${s.id}', 15)">+15m</button>
                                                <button class="btn-small btn-danger" onclick="OverviewPage.kickSession('${s.id}')">Kick</button>
                                            </div>
                                        </td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    `}
                </div>

                <!-- Children Status -->
                <div class="card">
                    <div class="card-header">
                        <h2>Children Status</h2>
                    </div>
                    ${children.length === 0 ? `
                        <div class="empty-state">
                            <p>No children configured</p>
                            <button onclick="Router.navigate('/children')">Add Child</button>
                        </div>
                    ` : `
                        <div class="card-grid">
                            ${children.map(c => {
                                const percent = Math.min(100, (c.used_today_min / c.daily_quota_min) * 100);
                                const progressClass = percent >= 90 ? 'danger' : percent >= 75 ? 'warning' : '';
                                const remainingClass = c.remaining_min < 15 ? 'danger' : c.remaining_min < 30 ? 'warning' : '';
                                return `
                                    <div class="card" style="margin-bottom: 0;">
                                        <div style="display: flex; justify-content: space-between; align-items: center;">
                                            <strong>${escapeHtml(c.name)}</strong>
                                            <span class="tag ${c.filter_mode === 'study' ? 'active' : ''}">${c.filter_mode}</span>
                                        </div>
                                        <div class="remaining-time-display ${remainingClass}" style="margin-top: 0.5rem; text-align: center;">
                                            <div class="remaining-time" style="font-size: 1.25rem;">${formatMinutes(c.remaining_min)}</div>
                                            <div style="font-size: 0.8rem; color: var(--text-secondary);">remaining</div>
                                        </div>
                                        <div class="progress-bar" style="margin-top: 0.5rem;">
                                            <div class="progress-fill ${progressClass}" style="width: ${percent}%"></div>
                                        </div>
                                        <div style="margin-top: 0.5rem; display: flex; justify-content: center; gap: 0.25rem;">
                                            <button class="btn-small btn-secondary" onclick="OverviewPage.adjustQuota('${c.id}', 15)">+15m</button>
                                            <button class="btn-small btn-secondary" onclick="OverviewPage.adjustQuota('${c.id}', 30)">+30m</button>
                                        </div>
                                    </div>
                                `;
                            }).join('')}
                        </div>
                    `}
                </div>
            `;

            // Auto-refresh every 30 seconds
            this.startAutoRefresh();

        } catch (error) {
            content.innerHTML = `<div class="error">Failed to load dashboard: ${error.message}</div>`;
        }
    },

    renderResourceBars(dashboard) {
        // Memory bar
        const memPercent = dashboard.memory_percent || 0;
        const memUsed = dashboard.memory_used_mb || dashboard.memory_usage_mb || 0;
        const memTotal = dashboard.memory_total_mb || 0;

        // Disk bar
        const diskPercent = dashboard.disk_used_percent || 0;

        return `
            <div class="resource-bar">
                <div class="resource-bar-header">
                    <span>Memory</span>
                    <span>${memUsed.toFixed(0)} / ${memTotal.toFixed(0)} MB (${memPercent.toFixed(0)}%)</span>
                </div>
                <div class="resource-bar-fill">
                    <div style="width: ${memPercent}%"></div>
                </div>
            </div>
            ${diskPercent > 0 ? `
                <div class="resource-bar">
                    <div class="resource-bar-header">
                        <span>Disk (/opt)</span>
                        <span>${diskPercent.toFixed(0)}%</span>
                    </div>
                    <div class="resource-bar-fill">
                        <div style="width: ${diskPercent}%"></div>
                    </div>
                </div>
            ` : ''}
        `;
    },

    async kickSession(id) {
        if (!confirm('Are you sure you want to disconnect this session?')) {
            return;
        }

        try {
            await API.kickSession(id);
            this.render();
        } catch (error) {
            alert('Failed to kick session: ' + error.message);
        }
    },

    async extendSession(id, minutes) {
        try {
            await API.post(`/api/sessions/${id}/extend`, { minutes });
            this.render();
        } catch (error) {
            alert('Failed to extend session: ' + error.message);
        }
    },

    async adjustQuota(childId, minutes) {
        try {
            await API.post(`/api/children/${childId}/adjust-quota`, { minutes });
            this.render();
        } catch (error) {
            alert('Failed to adjust quota: ' + error.message);
        }
    },

    startAutoRefresh() {
        this.stopAutoRefresh();
        this.refreshInterval = setInterval(() => {
            if (Router.currentPage === '/overview' || Router.currentPage === '/') {
                this.render();
            } else {
                this.stopAutoRefresh();
            }
        }, 30000);
    },

    stopAutoRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }
};
