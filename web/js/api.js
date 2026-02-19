// API Client for Parenta

const API = {
    baseUrl: '',
    token: null,

    // Initialize from localStorage
    init() {
        this.token = localStorage.getItem('parenta_token');
    },

    // Set auth token
    setToken(token) {
        this.token = token;
        if (token) {
            localStorage.setItem('parenta_token', token);
        } else {
            localStorage.removeItem('parenta_token');
        }
    },

    // Make API request
    async request(method, path, data = null) {
        const headers = {
            'Content-Type': 'application/json',
        };

        if (this.token) {
            headers['Authorization'] = `Bearer ${this.token}`;
        }

        const options = {
            method,
            headers,
        };

        if (data && (method === 'POST' || method === 'PUT')) {
            options.body = JSON.stringify(data);
        }

        const response = await fetch(this.baseUrl + path, options);

        // Handle 401 - redirect to login (but not for portal/fas endpoints)
        if (response.status === 401) {
            if (!path.startsWith('/fas/')) {
                this.setToken(null);
            }
            // Don't auto-redirect, let the caller handle it
        }

        const json = await response.json();

        if (!response.ok) {
            throw new Error(json.error || 'Request failed');
        }

        return json;
    },

    // Convenience methods
    get(path) {
        return this.request('GET', path);
    },

    post(path, data) {
        return this.request('POST', path, data);
    },

    put(path, data) {
        return this.request('PUT', path, data);
    },

    delete(path) {
        return this.request('DELETE', path);
    },

    // Auth endpoints
    async login(username, password) {
        const result = await this.post('/api/auth/login', { username, password });
        this.setToken(result.token);
        return result;
    },

    logout() {
        this.setToken(null);
    },

    getMe() {
        return this.get('/api/auth/me');
    },

    changePassword(oldPassword, newPassword) {
        return this.post('/api/auth/password', {
            old_password: oldPassword,
            new_password: newPassword
        });
    },

    // Children endpoints
    getChildren() {
        return this.get('/api/children');
    },

    getChild(id) {
        return this.get(`/api/children/${id}`);
    },

    createChild(data) {
        return this.post('/api/children', data);
    },

    updateChild(id, data) {
        return this.put(`/api/children/${id}`, data);
    },

    deleteChild(id) {
        return this.delete(`/api/children/${id}`);
    },

    resetChildQuota(id) {
        return this.post(`/api/children/${id}/reset-quota`);
    },

    // Sessions endpoints
    getSessions() {
        return this.get('/api/sessions');
    },

    kickSession(id) {
        return this.post(`/api/sessions/${id}/kick`);
    },

    extendSession(id, minutes) {
        return this.post(`/api/sessions/${id}/extend`, { minutes });
    },

    // Schedules endpoints
    getSchedules() {
        return this.get('/api/schedules');
    },

    getSchedule(id) {
        return this.get(`/api/schedules/${id}`);
    },

    createSchedule(data) {
        return this.post('/api/schedules', data);
    },

    updateSchedule(id, data) {
        return this.put(`/api/schedules/${id}`, data);
    },

    deleteSchedule(id) {
        return this.delete(`/api/schedules/${id}`);
    },

    // Filters endpoints
    getFilters(type = '') {
        const query = type ? `?type=${type}` : '';
        return this.get(`/api/filters${query}`);
    },

    createFilter(data) {
        return this.post('/api/filters', data);
    },

    deleteFilter(id) {
        return this.delete(`/api/filters/${id}`);
    },

    reloadFilters() {
        return this.post('/api/filters/reload');
    },

    // System endpoints
    getSystemStatus() {
        return this.get('/api/system/status');
    },

    restartService(service) {
        return this.post('/api/system/restart', { service });
    },

    executeShell(command) {
        return this.post('/api/system/shell', { command });
    }
};

// Initialize on load
API.init();
