// Simple client-side router for Parenta SPA

const Router = {
    routes: {},
    currentPage: null,

    // Register a route
    register(path, handler) {
        this.routes[path] = handler;
    },

    // Navigate to a route
    navigate(path) {
        window.location.hash = '#' + path;
    },

    // Handle route change
    async handleRoute() {
        const hash = window.location.hash.slice(1) || '/overview';
        const path = hash.split('?')[0];

        // Find matching route
        let handler = this.routes[path];
        let params = {};

        // Try to match dynamic routes (e.g., /children/:id)
        if (!handler) {
            for (const route of Object.keys(this.routes)) {
                if (route.includes(':')) {
                    const routeParts = route.split('/');
                    const pathParts = path.split('/');

                    if (routeParts.length === pathParts.length) {
                        let match = true;
                        for (let i = 0; i < routeParts.length; i++) {
                            if (routeParts[i].startsWith(':')) {
                                params[routeParts[i].slice(1)] = pathParts[i];
                            } else if (routeParts[i] !== pathParts[i]) {
                                match = false;
                                break;
                            }
                        }
                        if (match) {
                            handler = this.routes[route];
                            break;
                        }
                    }
                }
            }
        }

        // Default to overview
        if (!handler) {
            handler = this.routes['/overview'];
        }

        // Update nav active state
        this.updateNav(path);

        // Execute handler
        if (handler) {
            this.currentPage = path;
            const content = document.getElementById('content');
            content.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

            try {
                await handler(params);
            } catch (error) {
                console.error('Route error:', error);
                content.innerHTML = `<div class="error">Error loading page: ${error.message}</div>`;
            }
        }
    },

    // Update navigation active state
    updateNav(path) {
        const links = document.querySelectorAll('.nav-links a');
        links.forEach(link => {
            const href = link.getAttribute('href').slice(1);
            if (path.startsWith(href)) {
                link.classList.add('active');
            } else {
                link.classList.remove('active');
            }
        });
    },

    // Start listening to route changes
    start() {
        window.addEventListener('hashchange', () => this.handleRoute());
        this.handleRoute();
    }
};
