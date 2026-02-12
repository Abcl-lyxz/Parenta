// Main App for Parenta

const App = {
    isAuthenticated: false,
    forcePasswordChange: false,

    // Initialize the app
    async init() {
        // Check if we have a token
        if (API.token) {
            try {
                const user = await API.getMe();
                this.isAuthenticated = true;
                this.forcePasswordChange = user.force_password_change;
            } catch (e) {
                // Token invalid
                API.logout();
            }
        }

        // Setup event listeners
        this.setupEventListeners();

        // Show appropriate UI
        this.updateUI();

        // Register routes
        this.registerRoutes();

        // Start router if authenticated
        if (this.isAuthenticated) {
            Router.start();
        }
    },

    // Setup event listeners
    setupEventListeners() {
        // Login form
        const loginForm = document.getElementById('login-form');
        loginForm.addEventListener('submit', (e) => this.handleLogin(e));

        // Logout button
        const logoutBtn = document.getElementById('logout-btn');
        logoutBtn.addEventListener('click', () => this.handleLogout());

        // Password change form
        const passwordForm = document.getElementById('password-form');
        passwordForm.addEventListener('submit', (e) => this.handlePasswordChange(e));
    },

    // Update UI based on auth state
    updateUI() {
        const loginContainer = document.getElementById('login-container');
        const mainContainer = document.getElementById('main-container');
        const passwordModal = document.getElementById('password-modal');

        if (this.isAuthenticated) {
            loginContainer.classList.add('hidden');
            mainContainer.classList.remove('hidden');

            if (this.forcePasswordChange) {
                passwordModal.classList.remove('hidden');
            } else {
                passwordModal.classList.add('hidden');
            }
        } else {
            loginContainer.classList.remove('hidden');
            mainContainer.classList.add('hidden');
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
            const result = await API.login(username, password);
            this.isAuthenticated = true;
            this.forcePasswordChange = result.force_password_change;
            errorEl.classList.add('hidden');
            this.updateUI();
            Router.start();
        } catch (error) {
            errorEl.textContent = error.message;
            errorEl.classList.remove('hidden');
        }
    },

    // Handle logout
    handleLogout() {
        API.logout();
        this.isAuthenticated = false;
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

    // Register routes
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
    App.init();
});
