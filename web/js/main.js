/**
 * gstream.io - Main JavaScript
 * Handles interactions, animations, and UI enhancements
 */

(function() {
    'use strict';

    // ===== Navbar Scroll Effect =====
    const navbar = document.querySelector('.navbar');
    let lastScrollTop = 0;

    function handleNavbarScroll() {
        const scrollTop = window.pageYOffset || document.documentElement.scrollTop;

        if (scrollTop > 50) {
            navbar.classList.add('scrolled');
        } else {
            navbar.classList.remove('scrolled');
        }

        lastScrollTop = scrollTop;
    }

    window.addEventListener('scroll', handleNavbarScroll, { passive: true });

    // ===== Mobile Menu Toggle =====
    const mobileMenuToggle = document.querySelector('.mobile-menu-toggle');
    const navLinks = document.querySelector('.nav-links');

    if (mobileMenuToggle) {
        mobileMenuToggle.addEventListener('click', function() {
            navLinks.classList.toggle('active');
            this.classList.toggle('active');

            // Toggle aria-expanded for accessibility
            const isExpanded = this.getAttribute('aria-expanded') === 'true';
            this.setAttribute('aria-expanded', !isExpanded);
        });
    }

    // Close mobile menu when clicking outside
    document.addEventListener('click', function(event) {
        if (!event.target.closest('.navbar')) {
            navLinks?.classList.remove('active');
            mobileMenuToggle?.classList.remove('active');
        }
    });

    // Close mobile menu when clicking a link
    document.querySelectorAll('.nav-links a').forEach(link => {
        link.addEventListener('click', function() {
            navLinks?.classList.remove('active');
            mobileMenuToggle?.classList.remove('active');
        });
    });

    // ===== Code Tab Switching =====
    const codeTabs = document.querySelectorAll('.code-tab');
    const codePanels = document.querySelectorAll('.code-panel');

    codeTabs.forEach(tab => {
        tab.addEventListener('click', function() {
            const targetTab = this.dataset.tab;

            // Remove active class from all tabs
            codeTabs.forEach(t => t.classList.remove('active'));

            // Add active class to clicked tab
            this.classList.add('active');

            // Hide all panels
            codePanels.forEach(panel => panel.classList.remove('active'));

            // Show target panel
            const targetPanel = document.querySelector(`[data-panel="${targetTab}"]`);
            if (targetPanel) {
                targetPanel.classList.add('active');
            }
        });
    });

    // ===== Copy to Clipboard =====
    const copyButtons = document.querySelectorAll('.copy-btn');

    copyButtons.forEach(button => {
        button.addEventListener('click', function() {
            const commandElement = this.previousElementSibling;
            const textToCopy = commandElement.textContent;

            // Copy to clipboard
            navigator.clipboard.writeText(textToCopy).then(() => {
                // Visual feedback
                const originalHTML = this.innerHTML;
                this.innerHTML = `
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none">
                        <path d="M20 6L9 17L4 12" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                    </svg>
                `;

                // Reset after 2 seconds
                setTimeout(() => {
                    this.innerHTML = originalHTML;
                }, 2000);
            }).catch(err => {
                console.error('Failed to copy:', err);
            });
        });
    });

    // ===== Smooth Scroll for Anchor Links =====
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
        anchor.addEventListener('click', function(e) {
            const targetId = this.getAttribute('href');

            // Skip if it's just "#"
            if (targetId === '#') {
                e.preventDefault();
                return;
            }

            const targetElement = document.querySelector(targetId);

            if (targetElement) {
                e.preventDefault();

                const navbarHeight = navbar.offsetHeight;
                const targetPosition = targetElement.offsetTop - navbarHeight;

                window.scrollTo({
                    top: targetPosition,
                    behavior: 'smooth'
                });
            }
        });
    });

    // ===== Intersection Observer for Animations =====
    const observerOptions = {
        threshold: 0.1,
        rootMargin: '0px 0px -50px 0px'
    };

    const fadeInObserver = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.style.opacity = '1';
                entry.target.style.transform = 'translateY(0)';
            }
        });
    }, observerOptions);

    // Observe elements for fade-in animation
    const animateElements = document.querySelectorAll('.feature-card, .metric-card, .enterprise-feature, .community-card');

    animateElements.forEach((element, index) => {
        element.style.opacity = '0';
        element.style.transform = 'translateY(30px)';
        element.style.transition = `opacity 0.6s ease-out ${index * 0.1}s, transform 0.6s ease-out ${index * 0.1}s`;
        fadeInObserver.observe(element);
    });

    // ===== Performance Stats Counter Animation =====
    function animateValue(element, start, end, duration, suffix = '') {
        const startTime = performance.now();
        const isDecimal = end < 1;

        function update(currentTime) {
            const elapsed = currentTime - startTime;
            const progress = Math.min(elapsed / duration, 1);

            // Easing function (ease-out)
            const easeOut = 1 - Math.pow(1 - progress, 3);
            const current = start + (end - start) * easeOut;

            if (isDecimal) {
                element.textContent = current.toFixed(1) + suffix;
            } else {
                element.textContent = Math.floor(current).toLocaleString() + suffix;
            }

            if (progress < 1) {
                requestAnimationFrame(update);
            }
        }

        requestAnimationFrame(update);
    }

    // ===== Lazy Loading for Performance =====
    if ('IntersectionObserver' in window) {
        const lazyImages = document.querySelectorAll('img[data-src]');

        const imageObserver = new IntersectionObserver((entries) => {
            entries.forEach(entry => {
                if (entry.isIntersecting) {
                    const img = entry.target;
                    img.src = img.dataset.src;
                    img.removeAttribute('data-src');
                    imageObserver.unobserve(img);
                }
            });
        });

        lazyImages.forEach(img => imageObserver.observe(img));
    }

    // ===== Preload Critical Resources =====


    // ===== Performance Monitoring =====
    if ('PerformanceObserver' in window) {
        // Monitor Largest Contentful Paint (LCP)
        try {
            const lcpObserver = new PerformanceObserver((list) => {
                const entries = list.getEntries();
                const lastEntry = entries[entries.length - 1];
                console.log('LCP:', lastEntry.renderTime || lastEntry.loadTime);
            });
            lcpObserver.observe({ entryTypes: ['largest-contentful-paint'] });
        } catch (e) {
            // Silently fail if not supported
        }
    }

    // ===== Enhanced Accessibility =====
    // Keyboard navigation for tabs
    codeTabs.forEach((tab, index) => {
        tab.setAttribute('role', 'tab');
        tab.setAttribute('tabindex', index === 0 ? '0' : '-1');

        tab.addEventListener('keydown', function(e) {
            let targetTab;

            if (e.key === 'ArrowRight') {
                e.preventDefault();
                targetTab = this.nextElementSibling || codeTabs[0];
            } else if (e.key === 'ArrowLeft') {
                e.preventDefault();
                targetTab = this.previousElementSibling || codeTabs[codeTabs.length - 1];
            } else if (e.key === 'Home') {
                e.preventDefault();
                targetTab = codeTabs[0];
            } else if (e.key === 'End') {
                e.preventDefault();
                targetTab = codeTabs[codeTabs.length - 1];
            }

            if (targetTab) {
                targetTab.click();
                targetTab.focus();
            }
        });
    });

    // ===== Focus Management =====
    // Show focus outline only when using keyboard
    let isUsingKeyboard = false;

    document.addEventListener('keydown', function(e) {
        if (e.key === 'Tab') {
            isUsingKeyboard = true;
            document.body.classList.add('keyboard-nav');
        }
    });

    document.addEventListener('mousedown', function() {
        isUsingKeyboard = false;
        document.body.classList.remove('keyboard-nav');
    });

    // ===== Dark Mode Support (Future Enhancement) =====
    // Check for user's preferred color scheme
    const prefersDarkMode = window.matchMedia('(prefers-color-scheme: dark)');

    function handleColorSchemeChange(e) {
        // Add dark mode class if user prefers dark mode
        if (e.matches) {
            document.body.classList.add('dark-mode');
        } else {
            document.body.classList.remove('dark-mode');
        }
    }

    // Listen for changes in color scheme preference
    prefersDarkMode.addEventListener('change', handleColorSchemeChange);

    // ===== Analytics Tracking (Placeholder) =====
    function trackEvent(category, action, label) {
        // Placeholder for analytics tracking
        // Replace with your analytics implementation
        if (typeof gtag !== 'undefined') {
            gtag('event', action, {
                'event_category': category,
                'event_label': label
            });
        }

        console.log('Event tracked:', { category, action, label });
    }

    // Track CTA button clicks
    document.querySelectorAll('.btn-primary, .btn-secondary').forEach(button => {
        button.addEventListener('click', function() {
            const text = this.textContent.trim();
            trackEvent('CTA', 'click', text);
        });
    });

    // Track external links
    document.querySelectorAll('a[target="_blank"]').forEach(link => {
        link.addEventListener('click', function() {
            const url = this.href;
            trackEvent('External Link', 'click', url);
        });
    });

    // ===== Service Worker Registration (PWA) =====
    if ('serviceWorker' in navigator) {
        // Uncomment when you want to enable PWA features
        // navigator.serviceWorker.register('/sw.js')
        //     .then(registration => console.log('Service Worker registered:', registration))
        //     .catch(error => console.log('Service Worker registration failed:', error));
    }

    // ===== Initialize on Load =====
    window.addEventListener('DOMContentLoaded', function() {
        // Add loaded class for animations
        document.body.classList.add('loaded');

        // Log initialization
        console.log('%c gstream.io ', 'background: linear-gradient(135deg, #667EEA, #764BA2); color: white; padding: 8px 16px; border-radius: 4px; font-weight: bold;');
        console.log('Next-generation distributed streaming platform');
    });

    // ===== Error Handling =====
    window.addEventListener('error', function(e) {
        console.error('Error caught:', e.error);
        // You can send this to an error tracking service
    });

    // ===== Utility Functions =====
    function debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    function throttle(func, limit) {
        let inThrottle;
        return function(...args) {
            if (!inThrottle) {
                func.apply(this, args);
                inThrottle = true;
                setTimeout(() => inThrottle = false, limit);
            }
        };
    }

    // Expose utility functions globally if needed
    window.gstream = {
        trackEvent,
        debounce,
        throttle
    };

})();
