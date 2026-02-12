// Children Page - Manage child accounts

const ChildrenPage = {
    async render() {
        const content = document.getElementById('content');

        try {
            const children = await API.getChildren();

            content.innerHTML = `
                <div class="page-header">
                    <h1>Children</h1>
                    <button onclick="ChildrenPage.showAddModal()">Add Child</button>
                </div>

                ${children.length === 0 ? `
                    <div class="empty-state">
                        <p>No children configured yet.</p>
                        <button onclick="ChildrenPage.showAddModal()">Add First Child</button>
                    </div>
                ` : `
                    <div class="card-grid">
                        ${children.map(c => {
                            const remaining = c.remaining_min;
                            const percent = Math.min(100, (c.used_today_min / c.daily_quota_min) * 100);
                            const progressClass = percent >= 90 ? 'danger' : percent >= 75 ? 'warning' : '';
                            const remainingClass = remaining < 15 ? 'danger' : remaining < 30 ? 'warning' : '';
                            return `
                                <div class="card">
                                    <div style="display: flex; justify-content: space-between; align-items: flex-start;">
                                        <div>
                                            <h3 style="margin-bottom: 0.25rem;">${escapeHtml(c.name)}</h3>
                                            <div style="color: var(--text-secondary); font-size: 0.85rem;">@${escapeHtml(c.username)}</div>
                                        </div>
                                        <span class="tag ${c.filter_mode === 'study' ? 'active' : ''}">${c.filter_mode}</span>
                                    </div>

                                    <!-- Prominent remaining time display -->
                                    <div class="remaining-time-display ${remainingClass}" style="margin-top: 1rem; text-align: center;">
                                        <div class="remaining-time">${formatMinutes(remaining)}</div>
                                        <div style="font-size: 0.85rem; color: var(--text-secondary);">remaining of ${formatMinutes(c.daily_quota_min)} daily</div>
                                    </div>

                                    <div style="margin-top: 0.75rem;">
                                        <div class="progress-bar">
                                            <div class="progress-fill ${progressClass}" style="width: ${percent}%"></div>
                                        </div>
                                    </div>

                                    <!-- Quick adjust buttons -->
                                    <div class="quick-adjust" style="margin-top: 0.75rem; display: flex; justify-content: center; gap: 0.5rem;">
                                        <button class="btn-small btn-secondary" onclick="ChildrenPage.adjustQuota('${c.id}', -30)" title="Remove 30 minutes">-30m</button>
                                        <button class="btn-small btn-secondary" onclick="ChildrenPage.adjustQuota('${c.id}', -15)" title="Remove 15 minutes">-15m</button>
                                        <button class="btn-small" onclick="ChildrenPage.adjustQuota('${c.id}', 15)" title="Add 15 minutes">+15m</button>
                                        <button class="btn-small" onclick="ChildrenPage.adjustQuota('${c.id}', 30)" title="Add 30 minutes">+30m</button>
                                    </div>

                                    <div style="margin-top: 0.75rem; font-size: 0.85rem; color: var(--text-secondary);">
                                        ${c.devices.length} device(s) registered
                                    </div>

                                    <div class="btn-group" style="margin-top: 1rem;">
                                        <button class="btn-small btn-secondary" onclick="ChildrenPage.showEditModal('${c.id}')">Edit</button>
                                        <button class="btn-small btn-secondary" onclick="ChildrenPage.resetQuota('${c.id}')">Reset</button>
                                        <button class="btn-small btn-danger" onclick="ChildrenPage.deleteChild('${c.id}')">Delete</button>
                                    </div>
                                </div>
                            `;
                        }).join('')}
                    </div>
                `}

                <!-- Add/Edit Modal -->
                <div id="child-modal" class="modal-overlay hidden">
                    <div class="modal">
                        <h2 id="child-modal-title">Add Child</h2>
                        <form id="child-form">
                            <input type="hidden" id="child-id">

                            <div class="form-row">
                                <div>
                                    <label for="child-name">Display Name</label>
                                    <input type="text" id="child-name" required>
                                </div>
                                <div>
                                    <label for="child-username">Username</label>
                                    <input type="text" id="child-username" required>
                                </div>
                            </div>

                            <label for="child-password">Password <span id="password-hint" style="color: var(--text-secondary); font-weight: normal;"></span></label>
                            <input type="password" id="child-password">

                            <label>Daily Quota</label>
                            <div class="quota-presets" style="margin-bottom: 0.5rem;">
                                <button type="button" class="quota-preset" data-minutes="30" onclick="ChildrenPage.setQuotaPreset(30)">30m</button>
                                <button type="button" class="quota-preset" data-minutes="60" onclick="ChildrenPage.setQuotaPreset(60)">1hr</button>
                                <button type="button" class="quota-preset" data-minutes="120" onclick="ChildrenPage.setQuotaPreset(120)">2hr</button>
                                <button type="button" class="quota-preset" data-minutes="180" onclick="ChildrenPage.setQuotaPreset(180)">3hr</button>
                                <button type="button" class="quota-preset" data-minutes="240" onclick="ChildrenPage.setQuotaPreset(240)">4hr</button>
                            </div>
                            <input type="number" id="child-quota" min="15" max="720" value="120" required style="width: 100px;">
                            <span style="color: var(--text-secondary); margin-left: 0.5rem;">minutes</span>

                            <div style="margin-top: 1rem;">
                                <label for="child-mode">Filter Mode</label>
                                <select id="child-mode">
                                    <option value="normal">Normal (Blacklist)</option>
                                    <option value="study">Study (Whitelist)</option>
                                </select>
                            </div>

                            <div id="child-error" class="error hidden"></div>

                            <div class="form-actions">
                                <button type="button" class="btn-secondary" onclick="ChildrenPage.hideModal()">Cancel</button>
                                <button type="submit">Save</button>
                            </div>
                        </form>
                    </div>
                </div>
            `;

            // Setup form handler
            document.getElementById('child-form').addEventListener('submit', (e) => this.handleSubmit(e));

            // Setup quota input listener to highlight matching preset
            document.getElementById('child-quota').addEventListener('input', (e) => this.highlightQuotaPreset(parseInt(e.target.value)));

        } catch (error) {
            content.innerHTML = `<div class="error">Failed to load children: ${error.message}</div>`;
        }
    },

    setQuotaPreset(minutes) {
        document.getElementById('child-quota').value = minutes;
        this.highlightQuotaPreset(minutes);
    },

    highlightQuotaPreset(minutes) {
        document.querySelectorAll('.quota-preset').forEach(btn => {
            if (parseInt(btn.dataset.minutes) === minutes) {
                btn.classList.add('active');
            } else {
                btn.classList.remove('active');
            }
        });
    },

    async renderDetail(params) {
        const content = document.getElementById('content');
        const id = params.id;

        try {
            const child = await API.getChild(id);

            content.innerHTML = `
                <div class="page-header">
                    <h1>${escapeHtml(child.name)}</h1>
                    <button class="btn-secondary" onclick="Router.navigate('/children')">Back</button>
                </div>

                <div class="card">
                    <div class="card-header">
                        <h2>Devices</h2>
                        <button class="btn-small" onclick="ChildrenPage.showAddDeviceModal('${id}')">Add Device</button>
                    </div>
                    ${child.devices.length === 0 ? `
                        <div class="empty-state">
                            <p>No devices registered. Devices are added automatically when the child logs in.</p>
                        </div>
                    ` : `
                        <table>
                            <thead>
                                <tr>
                                    <th>MAC Address</th>
                                    <th>Name</th>
                                    <th>First Seen</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${child.devices.map(d => `
                                    <tr>
                                        <td><code>${d.mac}</code></td>
                                        <td>${escapeHtml(d.name || 'Unnamed')}</td>
                                        <td>${formatDate(d.first_seen)}</td>
                                        <td>
                                            <button class="btn-small btn-danger" onclick="ChildrenPage.removeDevice('${id}', '${d.mac}')">Remove</button>
                                        </td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    `}
                </div>
            `;

        } catch (error) {
            content.innerHTML = `<div class="error">Failed to load child: ${error.message}</div>`;
        }
    },

    showAddModal() {
        document.getElementById('child-modal-title').textContent = 'Add Child';
        document.getElementById('child-id').value = '';
        document.getElementById('child-name').value = '';
        document.getElementById('child-username').value = '';
        document.getElementById('child-password').value = '';
        document.getElementById('child-quota').value = '120';
        document.getElementById('child-mode').value = 'normal';
        document.getElementById('password-hint').textContent = '(required)';
        document.getElementById('child-password').required = true;
        document.getElementById('child-error').classList.add('hidden');
        document.getElementById('child-modal').classList.remove('hidden');
        this.highlightQuotaPreset(120);
    },

    async showEditModal(id) {
        try {
            const child = await API.getChild(id);

            document.getElementById('child-modal-title').textContent = 'Edit Child';
            document.getElementById('child-id').value = child.id;
            document.getElementById('child-name').value = child.name;
            document.getElementById('child-username').value = child.username;
            document.getElementById('child-password').value = '';
            document.getElementById('child-quota').value = child.daily_quota_min;
            document.getElementById('child-mode').value = child.filter_mode;
            document.getElementById('password-hint').textContent = '(leave blank to keep current)';
            document.getElementById('child-password').required = false;
            document.getElementById('child-error').classList.add('hidden');
            document.getElementById('child-modal').classList.remove('hidden');
            this.highlightQuotaPreset(child.daily_quota_min);
        } catch (error) {
            alert('Failed to load child: ' + error.message);
        }
    },

    hideModal() {
        document.getElementById('child-modal').classList.add('hidden');
    },

    async handleSubmit(e) {
        e.preventDefault();

        const id = document.getElementById('child-id').value;
        const data = {
            name: document.getElementById('child-name').value,
            username: document.getElementById('child-username').value,
            daily_quota_min: parseInt(document.getElementById('child-quota').value),
            filter_mode: document.getElementById('child-mode').value,
            is_active: true
        };

        const password = document.getElementById('child-password').value;
        if (password) {
            data.password = password;
        }

        const errorEl = document.getElementById('child-error');

        try {
            if (id) {
                await API.updateChild(id, data);
            } else {
                await API.createChild(data);
            }
            this.hideModal();
            this.render();
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    async adjustQuota(id, minutes) {
        try {
            await API.post(`/api/children/${id}/adjust-quota`, { minutes });
            this.render();
        } catch (error) {
            alert('Failed to adjust quota: ' + error.message);
        }
    },

    async resetQuota(id) {
        if (!confirm('Reset this child\'s daily quota to full?')) {
            return;
        }

        try {
            await API.resetChildQuota(id);
            this.render();
        } catch (error) {
            alert('Failed to reset quota: ' + error.message);
        }
    },

    async deleteChild(id) {
        if (!confirm('Are you sure you want to delete this child? This action cannot be undone.')) {
            return;
        }

        try {
            await API.deleteChild(id);
            this.render();
        } catch (error) {
            alert('Failed to delete child: ' + error.message);
        }
    },

    async removeDevice(childId, mac) {
        if (!confirm('Remove this device?')) {
            return;
        }

        try {
            await API.delete(`/api/children/${childId}/devices?mac=${encodeURIComponent(mac)}`);
            this.renderDetail({ id: childId });
        } catch (error) {
            alert('Failed to remove device: ' + error.message);
        }
    }
};
