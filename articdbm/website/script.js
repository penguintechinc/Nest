// ArticDBM Website JavaScript
document.addEventListener('DOMContentLoaded', function() {
    
    // Smooth scrolling for navigation links
    const navLinks = document.querySelectorAll('.nav-links a[href^="#"]');
    navLinks.forEach(link => {
        link.addEventListener('click', function(e) {
            e.preventDefault();
            const targetId = this.getAttribute('href').substring(1);
            const targetElement = document.getElementById(targetId);
            
            if (targetElement) {
                targetElement.scrollIntoView({
                    behavior: 'smooth',
                    block: 'start'
                });
            }
        });
    });
    
    // Animate stats numbers when they come into view
    const observerOptions = {
        root: null,
        rootMargin: '0px',
        threshold: 0.5
    };
    
    const animateStats = function(entries, observer) {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                const statNumbers = entry.target.querySelectorAll('.stat-number');
                statNumbers.forEach(statNumber => {
                    animateNumber(statNumber);
                });
                observer.unobserve(entry.target);
            }
        });
    };
    
    const statsObserver = new IntersectionObserver(animateStats, observerOptions);
    const statsSection = document.querySelector('.stats');
    if (statsSection) {
        statsObserver.observe(statsSection);
    }
    
    // Animate number counting
    function animateNumber(element) {
        const text = element.textContent;
        const hasK = text.includes('K');
        const hasMs = text.includes('ms');
        const hasPlus = text.includes('+');
        const hasLt = text.includes('<');
        
        let finalNumber;
        if (hasMs) {
            finalNumber = 1;
        } else if (hasK) {
            finalNumber = parseInt(text.replace(/[K+]/g, ''));
        } else if (hasPlus) {
            finalNumber = parseInt(text.replace('+', ''));
        } else {
            finalNumber = parseInt(text) || 0;
        }
        
        let current = 0;
        const increment = finalNumber / 50;
        const timer = setInterval(() => {
            current += increment;
            if (current >= finalNumber) {
                current = finalNumber;
                clearInterval(timer);
            }
            
            let displayText = Math.floor(current).toString();
            if (hasK) displayText += 'K';
            if (hasPlus) displayText += '+';
            if (hasMs) displayText = '<1ms';
            if (hasLt && !hasMs) displayText = '<' + displayText;
            
            element.textContent = displayText;
        }, 40);
    }
    
    // Add scroll effect to header
    let lastScrollY = window.scrollY;
    window.addEventListener('scroll', () => {
        const header = document.querySelector('.header');
        if (window.scrollY > 100) {
            header.style.background = 'rgba(248, 250, 252, 0.95)';
            header.style.boxShadow = '0 2px 20px rgba(0, 0, 0, 0.1)';
        } else {
            header.style.background = 'var(--snow-white)';
            header.style.boxShadow = 'none';
        }
        lastScrollY = window.scrollY;
    });
    
    // Add hover effects to feature cards
    const featureCards = document.querySelectorAll('.feature-card');
    featureCards.forEach(card => {
        card.addEventListener('mouseenter', function() {
            this.style.transform = 'translateY(-8px) scale(1.02)';
        });
        
        card.addEventListener('mouseleave', function() {
            this.style.transform = 'translateY(0) scale(1)';
        });
    });
    
    // Add click analytics (placeholder for future implementation)
    const ctaButtons = document.querySelectorAll('.btn-cta, .btn-hero');
    ctaButtons.forEach(button => {
        button.addEventListener('click', function() {
            // Analytics tracking would go here
            console.log('CTA clicked:', this.textContent);
        });
    });
    
    // Add copy functionality to code snippets
    const codeSnippets = document.querySelectorAll('.code-snippet pre code');
    codeSnippets.forEach(code => {
        const copyButton = document.createElement('button');
        copyButton.textContent = '📋';
        copyButton.style.cssText = `
            position: absolute;
            top: 10px;
            right: 10px;
            background: rgba(255, 255, 255, 0.1);
            border: none;
            color: white;
            padding: 5px 10px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
        `;
        
        const wrapper = code.parentElement.parentElement;
        wrapper.style.position = 'relative';
        wrapper.appendChild(copyButton);
        
        copyButton.addEventListener('click', function() {
            navigator.clipboard.writeText(code.textContent).then(() => {
                this.textContent = '✅';
                setTimeout(() => {
                    this.textContent = '📋';
                }, 1000);
            });
        });
    });
    
    // Lazy loading for architecture boxes
    const archBoxes = document.querySelectorAll('.arch-box');
    const archObserver = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.style.opacity = '1';
                entry.target.style.transform = 'translateY(0)';
            }
        });
    }, { threshold: 0.3 });
    
    archBoxes.forEach(box => {
        box.style.opacity = '0';
        box.style.transform = 'translateY(20px)';
        box.style.transition = 'opacity 0.6s ease, transform 0.6s ease';
        archObserver.observe(box);
    });
    
    // Add Easter egg for developers
    console.log(`
    🧊 ArticDBM - Arctic Database Manager
    
    Thanks for checking out the console! 
    
    If you're interested in contributing to ArticDBM,
    check out our GitHub: https://github.com/penguintechinc/articdbm
    
    Stay cool! ❄️
    `);
    
    // Simple performance monitoring
    if ('performance' in window) {
        window.addEventListener('load', function() {
            const loadTime = performance.timing.loadEventEnd - performance.timing.navigationStart;
            console.log(`Page loaded in ${loadTime}ms`);
        });
    }
});

// Add some Arctic-themed particle effect (optional)
function createSnowflakes() {
    const snowflakeCount = 50;
    
    for (let i = 0; i < snowflakeCount; i++) {
        const snowflake = document.createElement('div');
        snowflake.innerHTML = '❄️';
        snowflake.style.cssText = `
            position: fixed;
            top: -10px;
            left: ${Math.random() * 100}%;
            font-size: ${Math.random() * 10 + 10}px;
            opacity: ${Math.random() * 0.3 + 0.1};
            animation: snowfall ${Math.random() * 10 + 10}s linear infinite;
            pointer-events: none;
            z-index: -1;
        `;
        document.body.appendChild(snowflake);
        
        setTimeout(() => {
            snowflake.remove();
        }, (Math.random() * 10 + 10) * 1000);
    }
}

// Add CSS for snowfall animation
const style = document.createElement('style');
style.textContent = `
    @keyframes snowfall {
        0% {
            transform: translateY(-10px) rotate(0deg);
        }
        100% {
            transform: translateY(100vh) rotate(360deg);
        }
    }
`;
document.head.appendChild(style);

// Uncomment to enable snowflakes (might be too much for some users)
// createSnowflakes();
// setInterval(createSnowflakes, 5000);