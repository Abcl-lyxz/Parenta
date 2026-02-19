// Unified Portal App for Parenta
// Handles both admin (dashboard) and child (internet access) authentication

const PortalApp = {
    fasParams: {},
    userType: null, // 'admin', 'child', or null
    isAuthenticated: false,
    forcePasswordChange: false,
    childData: null,

    // Initialize the app
    async init() {
        // Parse URL parameters (FAS data from captive portal)
        this.parseFASParams();

        // Check for auth redirect (form submission result)
        this.checkAuthRedirect();

        // Check existing sessions
        await this.checkExistingSessions();

        // Setup event listeners
        this.setupEventListeners();

        // Show appropriate UI
        this.updateUI();

        // Register routes for admin dashboard
        this.registerRoutes();

        // Start router if admin authenticated
        if (this.isAuthenticated && this.userType === 'admin') {
            Router.start();
        }
    },

    // Parse FAS parameters from URL
    parseFASParams() {
        const params = new URLSearchParams(window.location.search);
        this.fasParams = {
            hid: params.get('hid') || '',
            mac: params.get('mac') || '',
            ip: params.get('ip') || '',
            authdir: params.get('authdir') || '',
            originurl: params.get('originurl') || ''
        };

        // Set hidden form fields
        Object.keys(this.fasParams).forEach(key => {
            const el = document.getElementById('fas-' + key);
            if (el) el.value = this.fasParams[key];
        });
    },

    // Check for auth redirect (from form submission)
    checkAuthRedirect() {
        const params = new URLSearchParams(window.location.search);
        const authType = params.get('auth_type');
        const token = params.get('token');
        const forceChange = params.get('force_password_change') === 'true';

        if (authType === 'admin' && token) {
            API.setToken(token);
            this.userType = 'admin';
            this.isAuthenticated = true;
            this.forcePasswordChange = forceChange;

            // Clean URL
            window.history.replaceState({}, '', '/portal');
        }
    },

    // Check existing sessions (admin JWT or child MAC)
    async checkExistingSessions() {
        // Already authenticated from redirect
        if (this.isAuthenticated) return;

        // Check admin JWT
        if (API.token) {
            try {
                const user = await API.getMe();
                this.userType = 'admin';
                this.isAuthenticated = true;
                this.forcePasswordChange = user.force_password_change;
                return;
            } catch (e) {
                API.logout();
            }
        }

        // Check child session by MAC (if we have MAC from captive portal)
        if (this.fasParams.mac) {
            try {
                const status = await API.get('/fas/status?mac=' + encodeURIComponent(this.fasParams.mac));
                this.userType = 'child';
                this.isAuthenticated = true;
                this.childData = status;
                return;
            } catch (e) {
                // No active child session
            }
        }
    },

    // Setup event listeners
    setupEventListeners() {
        // Login form
        const loginForm = document.getElementById('login-form');
        if (loginForm) {
            loginForm.addEventListener('submit', (e) => this.handleLogin(e));
        }

        // Logout button
        const logoutBtn = document.getElementById('logout-btn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', () => this.handleLogout());
        }

        // Password change form
        const passwordForm = document.getElementById('password-form');
        if (passwordForm) {
            passwordForm.addEventListener('submit', (e) => this.handlePasswordChange(e));
        }
    },

    // Update UI based on state
    updateUI() {
        const loadingContainer = document.getElementById('loading-container');
        const loginContainer = document.getElementById('login-container');
        const childStatusContainer = document.getElementById('child-status-container');
        const mainContainer = document.getElementById('main-container');
        const passwordModal = document.getElementById('password-modal');

        // Hide loading
        loadingContainer.classList.add('hidden');

        if (this.isAuthenticated) {
            if (this.userType === 'admin') {
                // Admin - show dashboard
                loginContainer.classList.add('hidden');
                childStatusContainer.classList.add('hidden');
                mainContainer.classList.remove('hidden');

                if (this.forcePasswordChange) {
                    passwordModal.classList.remove('hidden');
                } else {
                    passwordModal.classList.add('hidden');
                }
            } else if (this.userType === 'child') {
                // Child - show status
                loginContainer.classList.add('hidden');
                mainContainer.classList.add('hidden');
                passwordModal.classList.add('hidden');
                childStatusContainer.classList.remove('hidden');

                // Update child info
                if (this.childData) {
                    document.getElementById('child-name').textContent = this.childData.child_name;
                    document.getElementById('remaining-minutes').textContent = this.childData.remaining_minutes;
                    // Additional info
                    document.getElementById('child-ip').textContent = this.fasParams.ip || '-';
                    document.getElementById('wifi-name').textContent = 'Parenta';
                    document.getElementById('used-today').textContent = this.childData.used_today || 0;
                    document.getElementById('daily-quota').textContent = this.childData.daily_quota || 0;
                }
            }
        } else {
            // Not authenticated - show login
            loginContainer.classList.remove('hidden');
            mainContainer.classList.add('hidden');
            childStatusContainer.classList.add('hidden');
            passwordModal.classList.add('hidden');
        }
    },

    // Handle login
    async handleLogin(e) {
        e.preventDefault();

        const username = document.getElementById('username').value;
        const password = document.getElementById('password').value;
        const errorEl = document.getElementById('login-error');

        try {
            // Use unified auth endpoint
            const result = await API.post('/fas/auth', {
                username,
                password,
                hid: this.fasParams.hid,
                mac: this.fasParams.mac,
                ip: this.fasParams.ip,
                authdir: this.fasParams.authdir,
                originurl: this.fasParams.originurl
            });

            errorEl.classList.add('hidden');

            if (result.type === 'admin') {
                // Admin login
                API.setToken(result.token);
                this.userType = 'admin';
                this.isAuthenticated = true;
                this.forcePasswordChange = result.force_password_change;
                this.updateUI();
                Router.start();
            } else if (result.type === 'child') {
                // Child login
                this.userType = 'child';
                this.isAuthenticated = true;
                this.childData = result;

                // Redirect to original URL or show status
                if (result.redirect_url && result.redirect_url !== 'null' && result.redirect_url !== '') {
                    window.location.href = result.redirect_url;
                } else {
                    this.updateUI();
                }
            }
        } catch (error) {
            errorEl.textContent = error.message || 'Invalid username or password';
            errorEl.classList.remove('hidden');
        }
    },

    // Handle logout
    handleLogout() {
        API.logout();
        this.isAuthenticated = false;
        this.userType = null;
        this.forcePasswordChange = false;
        this.updateUI();
        window.location.hash = '';
    },

    // Handle password change
    async handlePasswordChange(e) {
        e.preventDefault();

        const oldPassword = document.getElementById('old-password').value;
        const newPassword = document.getElementById('new-password').value;
        const confirmPassword = document.getElementById('confirm-password').value;
        const errorEl = document.getElementById('password-error');

        if (newPassword !== confirmPassword) {
            errorEl.textContent = 'Passwords do not match';
            errorEl.classList.remove('hidden');
            return;
        }

        try {
            await API.changePassword(oldPassword, newPassword);
            this.forcePasswordChange = false;
            this.updateUI();
            Router.handleRoute();
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    // Register routes for admin dashboard
    registerRoutes() {
        Router.register('/overview', OverviewPage.render.bind(OverviewPage));
        Router.register('/children', ChildrenPage.render.bind(ChildrenPage));
        Router.register('/children/:id', ChildrenPage.renderDetail.bind(ChildrenPage));
        Router.register('/schedules', SchedulesPage.render.bind(SchedulesPage));
        Router.register('/filters', FiltersPage.render.bind(FiltersPage));
        Router.register('/system', SystemPage.render.bind(SystemPage));
    }
};

// Utility functions
function formatMinutes(minutes) {
    const hours = Math.floor(minutes / 60);
    const mins = minutes % 60;
    if (hours > 0) {
        return `${hours}h ${mins}m`;
    }
    return `${mins}m`;
}

function formatTime(date) {
    if (typeof date === 'string') {
        date = new Date(date);
    }
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatDate(date) {
    if (typeof date === 'string') {
        date = new Date(date);
    }
    return date.toLocaleDateString();
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Start app when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    PortalApp.init();

    // Dark mode toggle
    const themeToggle = document.getElementById('theme-toggle');
    const html = document.documentElement;

    // Load saved theme or use system preference
    const savedTheme = localStorage.getItem('theme');
    const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const initialTheme = savedTheme || (systemDark ? 'dark' : 'light');
    html.setAttribute('data-theme', initialTheme);

    if (themeToggle) {
        themeToggle.addEventListener('click', () => {
            const current = html.getAttribute('data-theme');
            const next = current === 'dark' ? 'light' : 'dark';
            html.setAttribute('data-theme', next);
            localStorage.setItem('theme', next);
        });
    }

    // Listen for system theme changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
        if (!localStorage.getItem('theme')) {
            html.setAttribute('data-theme', e.matches ? 'dark' : 'light');
        }
    });

    // Mobile nav toggle
    const navToggle = document.getElementById('nav-toggle');
    const navLinks = document.querySelector('.nav-links');
    if (navToggle && navLinks) {
        navToggle.addEventListener('click', () => {
            navLinks.classList.toggle('open');
        });

        // Close menu when link clicked
        navLinks.querySelectorAll('a').forEach(a => {
            a.addEventListener('click', () => {
                navLinks.classList.remove('open');
            });
        });
    }
});
