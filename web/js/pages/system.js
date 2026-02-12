// System Page - Status, commands, logs, and admin management

const SystemPage = {
    logRefreshInterval: null,

    async render() {
        const content = document.getElementById('content');

        try {
            const [status, admins] = await Promise.all([
                API.getSystemStatus(),
                API.get('/api/admins').catch(() => [])
            ]);

            content.innerHTML = `
                <div class="page-header">
                    <h1>System</h1>
                </div>

                <!-- Tabs -->
                <div class="tabs">
                    <div class="tab active" onclick="SystemPage.showTab('status')">Status</div>
                    <div class="tab" onclick="SystemPage.showTab('commands')">Commands</div>
                    <div class="tab" onclick="SystemPage.showTab('logs')">Logs</div>
                    <div class="tab" onclick="SystemPage.showTab('admins')">Admins</div>
                </div>

                <!-- Status Tab -->
                <div id="tab-status" class="tab-content">
                    <div class="stats-grid">
                        <div class="stat-card">
                            <div class="stat-value">${status.uptime}</div>
                            <div class="stat-label">Uptime</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${status.memory_usage_mb.toFixed(1)} MB</div>
                            <div class="stat-label">Memory Usage</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${status.active_sessions}</div>
                            <div class="stat-label">Active Sessions</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${status.total_children}</div>
                            <div class="stat-label">Total Children</div>
                        </div>
                    </div>

                    <div class="card">
                        <div class="card-header">
                            <h2>Services</h2>
                        </div>
                        <div class="service-status-grid">
                            <div class="service-status">
                                <span class="status-dot ${status.opennds_running ? 'active' : ''}"></span>
                                <div>
                                    <div>OpenNDS</div>
                                    <div style="font-size: 0.8rem; color: var(--text-secondary);">
                                        ${status.opennds_running ? 'Running' : 'Stopped'}
                                    </div>
                                </div>
                            </div>
                            <div class="service-status">
                                <span class="status-dot ${status.dnsmasq_running ? 'active' : ''}"></span>
                                <div>
                                    <div>Dnsmasq</div>
                                    <div style="font-size: 0.8rem; color: var(--text-secondary);">
                                        ${status.dnsmasq_running ? 'Running' : 'Stopped'}
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="btn-group">
                            <button class="btn-small btn-secondary" onclick="SystemPage.restartService('opennds')">Restart OpenNDS</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.restartService('dnsmasq')">Restart Dnsmasq</button>
                        </div>
                    </div>

                    <div class="card">
                        <div class="card-header">
                            <h2>Information</h2>
                        </div>
                        <table>
                            <tbody>
                                <tr><td>Version</td><td>${status.version}</td></tr>
                                <tr><td>Go Routines</td><td>${status.go_routines}</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <!-- Commands Tab -->
                <div id="tab-commands" class="tab-content hidden">
                    <div class="card">
                        <div class="card-header">
                            <h2>System Commands</h2>
                        </div>
                        <p style="font-size: 0.9rem; color: var(--text-secondary); margin-bottom: 1rem;">
                            Run diagnostic commands on the router. Only safe, read-only commands are allowed.
                        </p>
                        <div class="command-buttons">
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('uptime', [])">Uptime</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('free', ['-m'])">Memory</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('df', ['-h'])">Disk</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('ps', [])">Processes</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('ifconfig', [])">Network</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('iwinfo', [])">WiFi Info</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('ndsctl', ['status'])">NDS Status</button>
                            <button class="btn-small btn-secondary" onclick="SystemPage.runCommand('ndsctl', ['json'])">NDS Clients</button>
                        </div>
                        <div id="command-output" class="command-output hidden">
                            <pre></pre>
                        </div>
                    </div>
                </div>

                <!-- Logs Tab -->
                <div id="tab-logs" class="tab-content hidden">
                    <div class="card">
                        <div class="card-header">
                            <h2>System Logs</h2>
                        </div>
                        <div class="log-controls">
                            <select id="log-filter" onchange="SystemPage.loadLogs()">
                                <option value="">All Logs</option>
                                <option value="parenta">Parenta</option>
                                <option value="opennds">OpenNDS</option>
                                <option value="dnsmasq">Dnsmasq</option>
                            </select>
                            <button class="btn-small btn-secondary" onclick="SystemPage.loadLogs()">Refresh</button>
                            <label style="display: flex; align-items: center; gap: 0.25rem;">
                                <input type="checkbox" id="log-auto-refresh" onchange="SystemPage.toggleAutoRefresh()">
                                Auto-refresh
                            </label>
                        </div>
                        <div id="log-viewer" class="log-viewer">
                            <div class="log-content">Loading logs...</div>
                        </div>
                    </div>
                </div>

                <!-- Admins Tab -->
                <div id="tab-admins" class="tab-content hidden">
                    <div class="card">
                        <div class="card-header">
                            <h2>Administrators</h2>
                            <button class="btn-small" onclick="SystemPage.showAddAdminModal()">Add Admin</button>
                        </div>
                        <div class="admin-list">
                            ${admins.map(a => `
                                <div class="admin-item">
                                    <div class="admin-info">
                                        <span class="admin-name">${escapeHtml(a.display_name || a.username)}</span>
                                        <span class="admin-role">${a.role === 'super' ? 'Super Admin' : 'Admin'} - @${escapeHtml(a.username)}</span>
                                    </div>
                                    <div class="btn-group">
                                        <button class="btn-small btn-secondary" onclick="SystemPage.showEditAdminModal('${a.id}')">Edit</button>
                                        <button class="btn-small btn-secondary" onclick="SystemPage.resetAdminPassword('${a.id}')">Reset PW</button>
                                        ${admins.length > 1 ? `<button class="btn-small btn-danger" onclick="SystemPage.deleteAdmin('${a.id}')">Delete</button>` : ''}
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    </div>

                    <div class="card">
                        <div class="card-header">
                            <h2>Your Account</h2>
                        </div>
                        <button class="btn-secondary" onclick="SystemPage.showPasswordModal()">Change Password</button>
                    </div>
                </div>

                <!-- Password Modal -->
                <div id="system-password-modal" class="modal-overlay hidden">
                    <div class="modal">
                        <h2>Change Password</h2>
                        <form id="system-password-form">
                            <label for="sys-old-password">Current Password</label>
                            <input type="password" id="sys-old-password" required>

                            <label for="sys-new-password">New Password</label>
                            <input type="password" id="sys-new-password" required minlength="6">

                            <label for="sys-confirm-password">Confirm Password</label>
                            <input type="password" id="sys-confirm-password" required>

                            <div id="sys-password-error" class="error hidden"></div>

                            <div class="form-actions">
                                <button type="button" class="btn-secondary" onclick="SystemPage.hidePasswordModal()">Cancel</button>
                                <button type="submit">Change Password</button>
                            </div>
                        </form>
                    </div>
                </div>

                <!-- Add/Edit Admin Modal -->
                <div id="admin-modal" class="modal-overlay hidden">
                    <div class="modal">
                        <h2 id="admin-modal-title">Add Administrator</h2>
                        <form id="admin-form">
                            <input type="hidden" id="admin-id">

                            <label for="admin-username">Username</label>
                            <input type="text" id="admin-username" required>

                            <label for="admin-display-name">Display Name</label>
                            <input type="text" id="admin-display-name">

                            <label for="admin-password">Password <span id="admin-password-hint" style="color: var(--text-secondary); font-weight: normal;"></span></label>
                            <input type="password" id="admin-password">

                            <label for="admin-role">Role</label>
                            <select id="admin-role">
                                <option value="admin">Admin</option>
                                <option value="super">Super Admin (can manage other admins)</option>
                            </select>

                            <div id="admin-error" class="error hidden"></div>

                            <div class="form-actions">
                                <button type="button" class="btn-secondary" onclick="SystemPage.hideAdminModal()">Cancel</button>
                                <button type="submit">Save</button>
                            </div>
                        </form>
                    </div>
                </div>

                <!-- Reset Password Modal -->
                <div id="reset-password-modal" class="modal-overlay hidden">
                    <div class="modal">
                        <h2>Reset Password</h2>
                        <form id="reset-password-form">
                            <input type="hidden" id="reset-admin-id">

                            <label for="reset-new-password">New Password</label>
                            <input type="password" id="reset-new-password" required minlength="6">

                            <label for="reset-confirm-password">Confirm Password</label>
                            <input type="password" id="reset-confirm-password" required>

                            <div id="reset-password-error" class="error hidden"></div>

                            <div class="form-actions">
                                <button type="button" class="btn-secondary" onclick="SystemPage.hideResetPasswordModal()">Cancel</button>
                                <button type="submit">Reset Password</button>
                            </div>
                        </form>
                    </div>
                </div>
            `;

            // Setup form handlers
            document.getElementById('system-password-form').addEventListener('submit', (e) => this.handlePasswordChange(e));
            document.getElementById('admin-form').addEventListener('submit', (e) => this.handleAdminSubmit(e));
            document.getElementById('reset-password-form').addEventListener('submit', (e) => this.handleResetPassword(e));

        } catch (error) {
            content.innerHTML = `<div class="error">Failed to load system status: ${error.message}</div>`;
        }
    },

    showTab(tab) {
        // Update tab buttons
        document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        event.target.classList.add('active');

        // Update tab content
        document.querySelectorAll('.tab-content').forEach(c => c.classList.add('hidden'));
        document.getElementById(`tab-${tab}`).classList.remove('hidden');

        // Load logs when switching to logs tab
        if (tab === 'logs') {
            this.loadLogs();
        }

        // Clear log refresh when leaving logs tab
        if (tab !== 'logs' && this.logRefreshInterval) {
            clearInterval(this.logRefreshInterval);
            this.logRefreshInterval = null;
        }
    },

    // ============ Commands ============

    async runCommand(command, args) {
        const outputDiv = document.getElementById('command-output');
        const pre = outputDiv.querySelector('pre');

        outputDiv.classList.remove('hidden');
        pre.textContent = `Running: ${command} ${args.join(' ')}...`;

        try {
            const result = await API.post('/api/system/command', { command, args });
            pre.textContent = `$ ${result.command}\n\n${result.output}${result.error ? '\n\nError: ' + result.error : ''}`;
        } catch (error) {
            pre.textContent = `Error: ${error.message}`;
        }
    },

    // ============ Logs ============

    async loadLogs() {
        const filter = document.getElementById('log-filter').value;
        const viewer = document.getElementById('log-viewer');
        const logContent = viewer.querySelector('.log-content');

        try {
            const result = await API.get(`/api/system/logs?filter=${encodeURIComponent(filter)}&lines=100`);
            if (result.lines && result.lines.length > 0) {
                logContent.textContent = result.lines.join('\n');
                viewer.scrollTop = viewer.scrollHeight;
            } else {
                logContent.textContent = 'No logs found' + (filter ? ` for filter "${filter}"` : '');
            }
        } catch (error) {
            logContent.textContent = `Error loading logs: ${error.message}`;
        }
    },

    toggleAutoRefresh() {
        const checkbox = document.getElementById('log-auto-refresh');
        if (checkbox.checked) {
            this.loadLogs();
            this.logRefreshInterval = setInterval(() => this.loadLogs(), 10000);
        } else if (this.logRefreshInterval) {
            clearInterval(this.logRefreshInterval);
            this.logRefreshInterval = null;
        }
    },

    // ============ Services ============

    async restartService(service) {
        if (!confirm(`Restart ${service}? This may briefly interrupt connections.`)) {
            return;
        }

        try {
            await API.restartService(service);
            alert(`${service} restarted successfully`);
            this.render();
        } catch (error) {
            alert(`Failed to restart ${service}: ${error.message}`);
        }
    },

    // ============ Password Change ============

    showPasswordModal() {
        document.getElementById('sys-old-password').value = '';
        document.getElementById('sys-new-password').value = '';
        document.getElementById('sys-confirm-password').value = '';
        document.getElementById('sys-password-error').classList.add('hidden');
        document.getElementById('system-password-modal').classList.remove('hidden');
    },

    hidePasswordModal() {
        document.getElementById('system-password-modal').classList.add('hidden');
    },

    async handlePasswordChange(e) {
        e.preventDefault();

        const oldPassword = document.getElementById('sys-old-password').value;
        const newPassword = document.getElementById('sys-new-password').value;
        const confirmPassword = document.getElementById('sys-confirm-password').value;
        const errorEl = document.getElementById('sys-password-error');

        if (newPassword !== confirmPassword) {
            errorEl.textContent = 'Passwords do not match';
            errorEl.classList.remove('hidden');
            return;
        }

        try {
            await API.changePassword(oldPassword, newPassword);
            this.hidePasswordModal();
            alert('Password changed successfully');
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    // ============ Admin Management ============

    showAddAdminModal() {
        document.getElementById('admin-modal-title').textContent = 'Add Administrator';
        document.getElementById('admin-id').value = '';
        document.getElementById('admin-username').value = '';
        document.getElementById('admin-username').disabled = false;
        document.getElementById('admin-display-name').value = '';
        document.getElementById('admin-password').value = '';
        document.getElementById('admin-password').required = true;
        document.getElementById('admin-password-hint').textContent = '(required)';
        document.getElementById('admin-role').value = 'admin';
        document.getElementById('admin-error').classList.add('hidden');
        document.getElementById('admin-modal').classList.remove('hidden');
    },

    async showEditAdminModal(id) {
        try {
            const admin = await API.get(`/api/admins/${id}`);

            document.getElementById('admin-modal-title').textContent = 'Edit Administrator';
            document.getElementById('admin-id').value = admin.id;
            document.getElementById('admin-username').value = admin.username;
            document.getElementById('admin-username').disabled = true;
            document.getElementById('admin-display-name').value = admin.display_name || '';
            document.getElementById('admin-password').value = '';
            document.getElementById('admin-password').required = false;
            document.getElementById('admin-password-hint').textContent = '(leave blank to keep current)';
            document.getElementById('admin-role').value = admin.role;
            document.getElementById('admin-error').classList.add('hidden');
            document.getElementById('admin-modal').classList.remove('hidden');
        } catch (error) {
            alert('Failed to load admin: ' + error.message);
        }
    },

    hideAdminModal() {
        document.getElementById('admin-modal').classList.add('hidden');
    },

    async handleAdminSubmit(e) {
        e.preventDefault();

        const id = document.getElementById('admin-id').value;
        const errorEl = document.getElementById('admin-error');

        try {
            if (id) {
                // Update existing
                await API.put(`/api/admins/${id}`, {
                    display_name: document.getElementById('admin-display-name').value,
                    role: document.getElementById('admin-role').value
                });
            } else {
                // Create new
                const password = document.getElementById('admin-password').value;
                if (!password || password.length < 6) {
                    errorEl.textContent = 'Password must be at least 6 characters';
                    errorEl.classList.remove('hidden');
                    return;
                }
                await API.post('/api/admins', {
                    username: document.getElementById('admin-username').value,
                    password: password,
                    display_name: document.getElementById('admin-display-name').value,
                    role: document.getElementById('admin-role').value
                });
            }
            this.hideAdminModal();
            this.render();
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    resetAdminPassword(id) {
        document.getElementById('reset-admin-id').value = id;
        document.getElementById('reset-new-password').value = '';
        document.getElementById('reset-confirm-password').value = '';
        document.getElementById('reset-password-error').classList.add('hidden');
        document.getElementById('reset-password-modal').classList.remove('hidden');
    },

    hideResetPasswordModal() {
        document.getElementById('reset-password-modal').classList.add('hidden');
    },

    async handleResetPassword(e) {
        e.preventDefault();

        const id = document.getElementById('reset-admin-id').value;
        const newPassword = document.getElementById('reset-new-password').value;
        const confirmPassword = document.getElementById('reset-confirm-password').value;
        const errorEl = document.getElementById('reset-password-error');

        if (newPassword !== confirmPassword) {
            errorEl.textContent = 'Passwords do not match';
            errorEl.classList.remove('hidden');
            return;
        }

        try {
            await API.post(`/api/admins/${id}/reset-password`, { new_password: newPassword });
            this.hideResetPasswordModal();
            alert('Password reset successfully. User will be required to change it on next login.');
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    async deleteAdmin(id) {
        if (!confirm('Are you sure you want to delete this administrator?')) {
            return;
        }

        try {
            await API.delete(`/api/admins/${id}`);
            this.render();
        } catch (error) {
            alert('Failed to delete admin: ' + error.message);
        }
    }
};
