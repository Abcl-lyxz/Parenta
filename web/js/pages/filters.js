// Filters Page - Manage domain filtering rules

const FiltersPage = {
    currentTab: 'blacklist',

    async render() {
        const content = document.getElementById('content');

        try {
            const filters = await API.getFilters();
            const blacklist = filters.filter(f => f.rule_type === 'blacklist');
            const whitelist = filters.filter(f => f.rule_type === 'whitelist');

            content.innerHTML = `
                <div class="page-header">
                    <h1>Filtering</h1>
                    <div class="btn-group">
                        <button class="btn-secondary" onclick="FiltersPage.reloadFilters()">Apply Changes</button>
                        <button onclick="FiltersPage.showAddModal()">Add Rule</button>
                    </div>
                </div>

                <div class="card">
                    <div class="tabs">
                        <div class="tab ${this.currentTab === 'blacklist' ? 'active' : ''}" onclick="FiltersPage.switchTab('blacklist')">
                            Blacklist (${blacklist.length})
                        </div>
                        <div class="tab ${this.currentTab === 'whitelist' ? 'active' : ''}" onclick="FiltersPage.switchTab('whitelist')">
                            Whitelist (${whitelist.length})
                        </div>
                    </div>

                    <div id="filter-list">
                        ${this.renderFilterList(this.currentTab === 'blacklist' ? blacklist : whitelist)}
                    </div>
                </div>

                <div class="card" style="margin-top: 1rem;">
                    <div class="card-header">
                        <h2>How Filtering Works</h2>
                    </div>
                    <div style="font-size: 0.9rem; color: var(--text-secondary);">
                        <p><strong>Normal Mode:</strong> All domains are allowed except those in the blacklist.</p>
                        <p style="margin-top: 0.5rem;"><strong>Study Mode:</strong> All domains are blocked except those in the whitelist.</p>
                        <p style="margin-top: 0.5rem;"><em>Note: Changes take effect after clicking "Apply Changes".</em></p>
                    </div>
                </div>

                <!-- Add Modal -->
                <div id="filter-modal" class="modal-overlay hidden">
                    <div class="modal">
                        <h2>Add Filter Rule</h2>
                        <form id="filter-form">
                            <label for="filter-domain">Domain</label>
                            <input type="text" id="filter-domain" required placeholder="e.g., youtube.com">
                            <div style="font-size: 0.85rem; color: var(--text-secondary); margin-top: -0.75rem; margin-bottom: 1rem;">
                                Enter domain without http:// or www. Use *.domain.com for subdomains.
                            </div>

                            <label for="filter-type">Rule Type</label>
                            <select id="filter-type">
                                <option value="blacklist">Blacklist (Block in Normal mode)</option>
                                <option value="whitelist">Whitelist (Allow in Study mode)</option>
                            </select>

                            <label for="filter-category">Category (optional)</label>
                            <select id="filter-category">
                                <option value="">None</option>
                                <option value="social">Social Media</option>
                                <option value="games">Games</option>
                                <option value="video">Video/Streaming</option>
                                <option value="education">Education</option>
                                <option value="other">Other</option>
                            </select>

                            <div id="filter-error" class="error hidden"></div>

                            <div class="form-actions">
                                <button type="button" class="btn-secondary" onclick="FiltersPage.hideModal()">Cancel</button>
                                <button type="submit">Add</button>
                            </div>
                        </form>
                    </div>
                </div>

                <!-- Bulk Add Modal -->
                <div id="bulk-modal" class="modal-overlay hidden">
                    <div class="modal">
                        <h2>Bulk Add Domains</h2>
                        <form id="bulk-form">
                            <label for="bulk-domains">Domains (one per line)</label>
                            <textarea id="bulk-domains" rows="10" required placeholder="youtube.com&#10;facebook.com&#10;instagram.com"></textarea>

                            <label for="bulk-type">Rule Type</label>
                            <select id="bulk-type">
                                <option value="blacklist">Blacklist</option>
                                <option value="whitelist">Whitelist</option>
                            </select>

                            <div id="bulk-error" class="error hidden"></div>

                            <div class="form-actions">
                                <button type="button" class="btn-secondary" onclick="FiltersPage.hideBulkModal()">Cancel</button>
                                <button type="submit">Add All</button>
                            </div>
                        </form>
                    </div>
                </div>
            `;

            // Setup form handlers
            document.getElementById('filter-form').addEventListener('submit', (e) => this.handleSubmit(e));
            document.getElementById('bulk-form').addEventListener('submit', (e) => this.handleBulkSubmit(e));

        } catch (error) {
            content.innerHTML = `<div class="error">Failed to load filters: ${error.message}</div>`;
        }
    },

    renderFilterList(filters) {
        if (filters.length === 0) {
            return `
                <div class="empty-state">
                    <p>No ${this.currentTab} rules configured.</p>
                    <button onclick="FiltersPage.showAddModal()">Add First Rule</button>
                </div>
            `;
        }

        // Group by category
        const categories = {};
        filters.forEach(f => {
            const cat = f.category || 'Uncategorized';
            if (!categories[cat]) categories[cat] = [];
            categories[cat].push(f);
        });

        let html = '';
        for (const [category, rules] of Object.entries(categories)) {
            html += `
                <div style="margin-bottom: 1rem;">
                    <div style="font-weight: 600; font-size: 0.85rem; color: var(--text-secondary); margin-bottom: 0.5rem; text-transform: uppercase;">
                        ${category}
                    </div>
                    ${rules.map(f => `
                        <div class="list-item">
                            <code>${escapeHtml(f.domain)}</code>
                            <button class="btn-small btn-danger" onclick="FiltersPage.deleteFilter('${f.id}')">Remove</button>
                        </div>
                    `).join('')}
                </div>
            `;
        }

        return html;
    },

    switchTab(tab) {
        this.currentTab = tab;
        this.render();
    },

    showAddModal() {
        document.getElementById('filter-domain').value = '';
        document.getElementById('filter-type').value = this.currentTab;
        document.getElementById('filter-category').value = '';
        document.getElementById('filter-error').classList.add('hidden');
        document.getElementById('filter-modal').classList.remove('hidden');
    },

    hideModal() {
        document.getElementById('filter-modal').classList.add('hidden');
    },

    showBulkModal() {
        document.getElementById('bulk-domains').value = '';
        document.getElementById('bulk-type').value = this.currentTab;
        document.getElementById('bulk-error').classList.add('hidden');
        document.getElementById('bulk-modal').classList.remove('hidden');
    },

    hideBulkModal() {
        document.getElementById('bulk-modal').classList.add('hidden');
    },

    async handleSubmit(e) {
        e.preventDefault();

        const data = {
            domain: document.getElementById('filter-domain').value.trim().toLowerCase(),
            rule_type: document.getElementById('filter-type').value,
            category: document.getElementById('filter-category').value
        };

        // Clean domain
        data.domain = data.domain.replace(/^(https?:\/\/)?(www\.)?/, '');

        const errorEl = document.getElementById('filter-error');

        try {
            await API.createFilter(data);
            this.hideModal();
            this.render();
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    async handleBulkSubmit(e) {
        e.preventDefault();

        const domains = document.getElementById('bulk-domains').value
            .split('\n')
            .map(d => d.trim().toLowerCase().replace(/^(https?:\/\/)?(www\.)?/, ''))
            .filter(d => d.length > 0);

        const ruleType = document.getElementById('bulk-type').value;
        const errorEl = document.getElementById('bulk-error');

        try {
            for (const domain of domains) {
                await API.createFilter({
                    domain,
                    rule_type: ruleType,
                    category: ''
                });
            }
            this.hideBulkModal();
            this.render();
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    async deleteFilter(id) {
        try {
            await API.deleteFilter(id);
            this.render();
        } catch (error) {
            alert('Failed to delete filter: ' + error.message);
        }
    },

    async reloadFilters() {
        try {
            await API.reloadFilters();
            alert('Filters applied successfully!');
        } catch (error) {
            alert('Failed to apply filters: ' + error.message);
        }
    }
};
