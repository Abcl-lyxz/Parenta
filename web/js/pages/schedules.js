// Schedules Page - Manage time schedules with multi-day selection

const SchedulesPage = {
    async render() {
        const content = document.getElementById('content');

        try {
            const schedules = await API.getSchedules();

            const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

            content.innerHTML = `
                <div class="page-header">
                    <h1>Schedules</h1>
                    <button onclick="SchedulesPage.showAddModal()">Add Schedule</button>
                </div>

                ${schedules.length === 0 ? `
                    <div class="empty-state">
                        <p>No schedules created yet. Schedules define when children can access the internet.</p>
                        <button onclick="SchedulesPage.showAddModal()">Create First Schedule</button>
                    </div>
                ` : `
                    <div class="card-grid">
                        ${schedules.map(s => `
                            <div class="card">
                                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
                                    <h3>${escapeHtml(s.name)}</h3>
                                    ${s.is_default ? '<span class="tag active">Default</span>' : ''}
                                </div>

                                <div style="font-size: 0.9rem;">
                                    ${s.time_blocks.length === 0 ? `
                                        <div style="color: var(--text-secondary);">No time blocks defined</div>
                                    ` : `
                                        ${this.summarizeBlocks(s.time_blocks, days)}
                                    `}
                                </div>

                                <div class="btn-group" style="margin-top: 1rem;">
                                    <button class="btn-small btn-secondary" onclick="SchedulesPage.showEditModal('${s.id}')">Edit</button>
                                    <button class="btn-small btn-danger" onclick="SchedulesPage.deleteSchedule('${s.id}')">Delete</button>
                                </div>
                            </div>
                        `).join('')}
                    </div>
                `}

                <!-- Add/Edit Modal -->
                <div id="schedule-modal" class="modal-overlay hidden">
                    <div class="modal" style="max-width: 600px;">
                        <h2 id="schedule-modal-title">Add Schedule</h2>
                        <form id="schedule-form">
                            <input type="hidden" id="schedule-id">

                            <label for="schedule-name">Schedule Name</label>
                            <input type="text" id="schedule-name" required placeholder="e.g., School Days, Weekends">

                            <label>
                                <input type="checkbox" id="schedule-default">
                                Set as default schedule
                            </label>

                            <div style="margin-top: 1.5rem;">
                                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
                                    <label style="margin-bottom: 0; font-size: 1rem;">Time Blocks</label>
                                    <button type="button" class="btn-small" onclick="SchedulesPage.addTimeBlock()">+ Add Block</button>
                                </div>
                                <div id="time-blocks"></div>
                            </div>

                            <div id="schedule-error" class="error hidden"></div>

                            <div class="form-actions">
                                <button type="button" class="btn-secondary" onclick="SchedulesPage.hideModal()">Cancel</button>
                                <button type="submit">Save</button>
                            </div>
                        </form>
                    </div>
                </div>
            `;

            // Setup form handler
            document.getElementById('schedule-form').addEventListener('submit', (e) => this.handleSubmit(e));

        } catch (error) {
            content.innerHTML = `<div class="error">Failed to load schedules: ${error.message}</div>`;
        }
    },

    // Summarize blocks by grouping same times across days
    summarizeBlocks(blocks, days) {
        // Group blocks by time range and mode
        const groups = {};
        blocks.forEach(b => {
            const key = `${b.start_time}-${b.end_time}-${b.filter_mode}`;
            if (!groups[key]) {
                groups[key] = { ...b, days: [] };
            }
            groups[key].days.push(b.day_of_week);
        });

        // Format summary
        const summaries = Object.values(groups).slice(0, 3).map(g => {
            const dayNames = g.days.sort((a, b) => a - b).map(d => days[d].substring(0, 3));
            const daysStr = dayNames.length === 7 ? 'Every day' :
                dayNames.length === 5 && !g.days.includes(0) && !g.days.includes(6) ? 'Weekdays' :
                dayNames.length === 2 && g.days.includes(0) && g.days.includes(6) ? 'Weekends' :
                dayNames.join(', ');
            return `
                <div class="list-item">
                    <span>${daysStr}</span>
                    <span>${g.start_time} - ${g.end_time} ${g.filter_mode === 'study' ? '(Study)' : ''}</span>
                </div>
            `;
        }).join('');

        const remaining = Object.keys(groups).length - 3;
        return summaries + (remaining > 0 ? `<div style="color: var(--text-secondary); margin-top: 0.5rem;">+${remaining} more time ranges</div>` : '');
    },

    // Track time block groups (each can have multiple days)
    blockGroups: [],

    showAddModal() {
        document.getElementById('schedule-modal-title').textContent = 'Add Schedule';
        document.getElementById('schedule-id').value = '';
        document.getElementById('schedule-name').value = '';
        document.getElementById('schedule-default').checked = false;
        this.blockGroups = [];
        this.renderTimeBlocks();
        document.getElementById('schedule-error').classList.add('hidden');
        document.getElementById('schedule-modal').classList.remove('hidden');
    },

    async showEditModal(id) {
        try {
            const schedule = await API.getSchedule(id);

            document.getElementById('schedule-modal-title').textContent = 'Edit Schedule';
            document.getElementById('schedule-id').value = schedule.id;
            document.getElementById('schedule-name').value = schedule.name;
            document.getElementById('schedule-default').checked = schedule.is_default;

            // Convert flat blocks to groups (group by time/mode)
            this.blockGroups = this.blocksToGroups(schedule.time_blocks || []);
            this.renderTimeBlocks();
            document.getElementById('schedule-error').classList.add('hidden');
            document.getElementById('schedule-modal').classList.remove('hidden');
        } catch (error) {
            alert('Failed to load schedule: ' + error.message);
        }
    },

    // Convert flat time blocks to grouped format
    blocksToGroups(blocks) {
        const groups = {};
        blocks.forEach(b => {
            const key = `${b.start_time}-${b.end_time}-${b.filter_mode}`;
            if (!groups[key]) {
                groups[key] = {
                    days: [],
                    start_time: b.start_time,
                    end_time: b.end_time,
                    filter_mode: b.filter_mode
                };
            }
            if (!groups[key].days.includes(b.day_of_week)) {
                groups[key].days.push(b.day_of_week);
            }
        });
        return Object.values(groups);
    },

    // Convert groups back to flat blocks for API
    groupsToBlocks() {
        const blocks = [];
        this.blockGroups.forEach(g => {
            g.days.forEach(day => {
                blocks.push({
                    day_of_week: day,
                    start_time: g.start_time,
                    end_time: g.end_time,
                    filter_mode: g.filter_mode
                });
            });
        });
        return blocks;
    },

    hideModal() {
        document.getElementById('schedule-modal').classList.add('hidden');
    },

    addTimeBlock() {
        this.blockGroups.push({
            days: [1, 2, 3, 4, 5], // Default to weekdays
            start_time: '09:00',
            end_time: '17:00',
            filter_mode: 'normal'
        });
        this.renderTimeBlocks();
    },

    removeTimeBlock(index) {
        this.blockGroups.splice(index, 1);
        this.renderTimeBlocks();
    },

    renderTimeBlocks() {
        const container = document.getElementById('time-blocks');
        const dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

        if (this.blockGroups.length === 0) {
            container.innerHTML = '<div style="color: var(--text-secondary); padding: 1rem 0;">No time blocks. Add a block to define when internet access is allowed.</div>';
            return;
        }

        container.innerHTML = this.blockGroups.map((group, i) => `
            <div class="card" style="margin-bottom: 1rem; padding: 1rem;">
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
                    <strong>Time Block ${i + 1}</strong>
                    <button type="button" class="btn-small btn-danger" onclick="SchedulesPage.removeTimeBlock(${i})">Remove</button>
                </div>

                <!-- Day preset buttons -->
                <div class="day-presets">
                    <button type="button" onclick="SchedulesPage.setDayPreset(${i}, 'weekdays')">Weekdays</button>
                    <button type="button" onclick="SchedulesPage.setDayPreset(${i}, 'weekends')">Weekends</button>
                    <button type="button" onclick="SchedulesPage.setDayPreset(${i}, 'everyday')">Every Day</button>
                    <button type="button" onclick="SchedulesPage.setDayPreset(${i}, 'clear')">Clear</button>
                </div>

                <!-- Day checkboxes -->
                <div class="day-selector">
                    ${dayNames.map((d, di) => `
                        <label class="day-checkbox">
                            <input type="checkbox"
                                ${group.days.includes(di) ? 'checked' : ''}
                                onchange="SchedulesPage.toggleDay(${i}, ${di}, this.checked)">
                            ${d}
                        </label>
                    `).join('')}
                </div>

                <!-- Time range -->
                <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap;">
                    <input type="time" value="${group.start_time}"
                        onchange="SchedulesPage.updateGroup(${i}, 'start_time', this.value)"
                        style="width: auto; margin-bottom: 0;">
                    <span>to</span>
                    <input type="time" value="${group.end_time}"
                        onchange="SchedulesPage.updateGroup(${i}, 'end_time', this.value)"
                        style="width: auto; margin-bottom: 0;">
                    <select onchange="SchedulesPage.updateGroup(${i}, 'filter_mode', this.value)"
                        style="width: auto; margin-bottom: 0;">
                        <option value="normal" ${group.filter_mode === 'normal' ? 'selected' : ''}>Normal Mode</option>
                        <option value="study" ${group.filter_mode === 'study' ? 'selected' : ''}>Study Mode</option>
                    </select>
                </div>
            </div>
        `).join('');
    },

    setDayPreset(index, preset) {
        switch (preset) {
            case 'weekdays':
                this.blockGroups[index].days = [1, 2, 3, 4, 5];
                break;
            case 'weekends':
                this.blockGroups[index].days = [0, 6];
                break;
            case 'everyday':
                this.blockGroups[index].days = [0, 1, 2, 3, 4, 5, 6];
                break;
            case 'clear':
                this.blockGroups[index].days = [];
                break;
        }
        this.renderTimeBlocks();
    },

    toggleDay(groupIndex, dayIndex, checked) {
        const days = this.blockGroups[groupIndex].days;
        if (checked && !days.includes(dayIndex)) {
            days.push(dayIndex);
            days.sort((a, b) => a - b);
        } else if (!checked) {
            const idx = days.indexOf(dayIndex);
            if (idx > -1) days.splice(idx, 1);
        }
    },

    updateGroup(index, field, value) {
        this.blockGroups[index][field] = value;
    },

    async handleSubmit(e) {
        e.preventDefault();

        // Convert groups to flat blocks
        const blocks = this.groupsToBlocks();

        const id = document.getElementById('schedule-id').value;
        const data = {
            name: document.getElementById('schedule-name').value,
            is_default: document.getElementById('schedule-default').checked,
            time_blocks: blocks
        };

        const errorEl = document.getElementById('schedule-error');

        try {
            if (id) {
                await API.updateSchedule(id, data);
            } else {
                await API.createSchedule(data);
            }
            this.hideModal();
            this.render();
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    async deleteSchedule(id) {
        if (!confirm('Delete this schedule? Children using it will no longer have time restrictions.')) {
            return;
        }

        try {
            await API.deleteSchedule(id);
            this.render();
        } catch (error) {
            alert('Failed to delete schedule: ' + error.message);
        }
    }
};
