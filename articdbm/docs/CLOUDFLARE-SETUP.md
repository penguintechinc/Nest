# ☁️ Cloudflare Pages Setup Guide

This guide covers deploying the ArticDBM website and documentation to Cloudflare Pages.

## 🌐 Website Deployment (articdbm.penguintech.io)

### Prerequisites
- Cloudflare account
- GitHub repository access
- Custom domain configured in Cloudflare DNS

### Step 1: Connect Repository

1. **Login to Cloudflare Pages**
   - Go to [dash.cloudflare.com](https://dash.cloudflare.com)
   - Navigate to **Pages** in the sidebar
   - Click **Create a project**

2. **Connect to Git**
   - Select **Connect to Git**
   - Choose **GitHub** and authorize Cloudflare
   - Select your `articdbm/articdbm` repository
   - Choose the `main` branch (or `v0.1` if using feature branch)

### Step 2: Configure Build Settings

```yaml
Framework preset: None (Static HTML)
Build command: npm install && npm run build
Build output directory: website/dist
Root directory: (leave empty - use repository root)
```

### Step 3: Environment Variables

No environment variables are required for the static website.

### Step 4: Deploy Settings

```yaml
Production branch: main
Preview deployments: Enabled
Build system: V2 (recommended)
Node.js version: 18.x (not needed but good to set)
```

### Step 5: Custom Domain

1. **Add Custom Domain**
   - In your Pages project, go to **Custom domains**
   - Click **Set up a custom domain**
   - Enter `articdbm.penguintech.io`

2. **DNS Configuration**
   ```
   Type: CNAME
   Name: articdbm
   Target: your-project.pages.dev
   Proxy status: Proxied (orange cloud)
   ```

### Security Headers

The website includes `_headers` file with security configurations:

```
/*
  X-Frame-Options: DENY
  X-Content-Type-Options: nosniff
  X-XSS-Protection: 1; mode=block
  Referrer-Policy: strict-origin-when-cross-origin
  Permissions-Policy: geolocation=(), microphone=(), camera=()
  Content-Security-Policy: default-src 'self'; script-src 'self' 'unsafe-inline'
```

### URL Redirects

The `_redirects` file handles common redirects:

```
/docs/* https://github.com/penguintechinc/articdbm/blob/main/docs/:splat 301
/github https://github.com/penguintechinc/articdbm 301
/download https://github.com/penguintechinc/articdbm/archive/refs/heads/main.zip 301
```

## 📚 Documentation Site (docs.articdbm.penguintech.io)

The ArticDBM documentation is built with MkDocs Material and can be deployed to Cloudflare Pages for fast, global distribution.

### MkDocs Material on Cloudflare Pages

#### Step 1: Create Documentation Pages Project

1. **New Pages Project**
   - Go to Cloudflare Pages dashboard
   - Click **Create a project**
   - Connect to the same `articdbm/articdbm` repository
   - Use a different project name: `articdbm-docs`

2. **Build Configuration**
   ```yaml
   Framework preset: Other (or None)
   Build command: pip install -r requirements.txt && mkdocs build --strict
   Build output directory: site
   Root directory: (leave empty - uses repository root)
   Node.js version: Not needed (Python-based build)
   ```

#### Step 2: Python Runtime Configuration

The repository includes these files for proper Python environment:

- **`requirements.txt`** - MkDocs and extensions:
  ```txt
  mkdocs>=1.5.0
  mkdocs-material>=9.4.0
  mkdocs-git-revision-date-localized-plugin>=1.2.0
  mkdocs-minify-plugin>=0.7.0
  pymdown-extensions>=10.3.0
  ```

- **`runtime.txt`** - Python version:
  ```txt
  python-3.13
  ```

#### Step 3: Environment Variables

Set these in the Pages project settings:

```yaml
PYTHON_VERSION: 3.13
MKDOCS_STRICT: true  # Optional: fail build on warnings
```

#### Step 4: Custom Domain Setup

1. **Add Custom Domain**
   - In the docs Pages project: **Custom domains**
   - Add: `docs.articdbm.penguintech.io`

2. **DNS Configuration**
   ```dns
   Type: CNAME
   Name: docs
   Target: articdbm-docs.pages.dev
   Proxy status: Proxied (orange cloud)
   TTL: Auto
   ```

#### Step 5: MkDocs Configuration

The `mkdocs.yml` includes Arctic theme colors and features:

```yaml
theme:
  name: material
  palette:
    - media: "(prefers-color-scheme: light)"
      scheme: default
      primary: blue      # Arctic blue
      accent: cyan       # Ice blue
    - media: "(prefers-color-scheme: dark)"
      scheme: slate  
      primary: blue
      accent: cyan

nav:
  - Home: index.md
  - Getting Started:
    - Overview: ../README.md
    - Usage Guide: USAGE.md
  - Architecture:
    - System Design: architecture.md
  - Deployment:
    - Kubernetes: kubernetes.md
    - Cloudflare Setup: cloudflare-setup.md
  - Community:
    - Contributing: CONTRIBUTING.md
```

#### Features Enabled

- **Search**: Full-text search across all documentation
- **Navigation**: Tabbed navigation with auto-expand
- **Code Highlighting**: Syntax highlighting for all languages
- **Mermaid Diagrams**: Architecture diagrams support
- **Git Integration**: Last modified dates from Git history
- **Responsive Design**: Mobile-optimized layout
- **Dark Mode**: Automatic theme switching

### Option 2: GitHub Pages Integration

Alternatively, you can deploy MkDocs to GitHub Pages and use Cloudflare DNS:

1. **GitHub Actions Workflow** (`.github/workflows/docs.yml`)
   ```yaml
   name: Deploy Documentation
   on:
     push:
       branches: [main]
       paths: [docs/**, mkdocs.yml]
   
   jobs:
     deploy:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         - uses: actions/setup-python@v4
           with:
             python-version: 3.11
         - run: pip install mkdocs-material pymdown-extensions
         - run: mkdocs gh-deploy --force
   ```

2. **Cloudflare DNS**
   ```
   Type: CNAME  
   Name: docs
   Target: articdbm.github.io
   Proxy status: Proxied
   ```

## 🎨 Customization

### Website Theme Colors

The Arctic theme uses these CSS custom properties:

```css
:root {
    --primary-blue: #1e3a8a;
    --light-blue: #3b82f6; 
    --ice-blue: #dbeafe;
    --frost-white: #f8fafc;
    --snow-white: #ffffff;
    --arctic-gray: #64748b;
    --deep-blue: #1e293b;
    --accent-cyan: #06b6d4;
}
```

### Documentation Theme

MkDocs Material configuration in `mkdocs.yml`:

```yaml
theme:
  name: material
  palette:
    - media: "(prefers-color-scheme: light)"
      scheme: default
      primary: blue
      accent: cyan
    - media: "(prefers-color-scheme: dark)" 
      scheme: slate
      primary: blue
      accent: cyan
```

## 📈 Analytics & Monitoring

### Cloudflare Web Analytics

1. **Enable Web Analytics**
   - Go to **Analytics** > **Web Analytics**
   - Add your domain: `articdbm.penguintech.io`
   - Copy the beacon code

2. **Add to Website**
   Add before closing `</body>` tag in `index.html`:
   ```html
   <!-- Cloudflare Web Analytics -->
   <script defer src='https://static.cloudflareinsights.com/beacon.min.js' 
           data-cf-beacon='{"token": "your-token-here"}'></script>
   ```

### Performance Monitoring

Cloudflare automatically provides:
- **Core Web Vitals** monitoring
- **Page load times** analytics  
- **Geographic performance** data
- **Security threat** blocking

## 🔒 Security Configuration

### SSL/TLS Settings

1. **SSL/TLS Overview**
   - Set encryption mode to **Full (strict)**
   - Enable **Always Use HTTPS**
   - Set **Minimum TLS Version** to 1.2

2. **Security Features**
   ```yaml
   HSTS: Enabled (6 months)
   DNSSEC: Enabled
   Bot Fight Mode: Enabled
   Browser Integrity Check: Enabled
   ```

### Firewall Rules

Add firewall rules for enhanced security:

```yaml
# Block common attack patterns
(http.request.uri.path contains "/admin" and cf.threat_score > 10)

# Rate limiting for API endpoints  
(http.request.uri.path contains "/api/" and rate(1m) > 100)

# Geographic restrictions (if needed)
(ip.geoip.country ne "US" and http.request.uri.path contains "/download")
```

## 🚀 Deployment Workflow

### Automatic Deployments

Every push to main branch triggers:

1. **Build Process**
   - Cloudflare fetches latest code
   - Builds static assets (if needed)
   - Runs security scans

2. **Preview Deployments**
   - Pull requests get preview URLs
   - Test changes before merging
   - Automatic cleanup after merge

3. **Production Deployment**
   - Atomic deployments (all-or-nothing)
   - Instant global distribution
   - Automatic rollback on errors

### Manual Deployment

For manual deployments:

```bash
# Build locally (optional)
cd website
python -m http.server 8080  # Test locally

# Deploy via CLI (optional)
npx wrangler pages publish website --project-name=articdbm

# Or use Git workflow (recommended)
git push origin main
```

## 🌍 Global Distribution

Cloudflare Pages provides:

- **275+ Edge Locations** worldwide
- **Automatic caching** with smart rules
- **HTTP/3 and Brotli** compression
- **IPv6 support** enabled by default

### Performance Optimization

```yaml
# Automatic optimizations:
Minification: HTML, CSS, JS
Image optimization: WebP conversion
Caching: Static assets (1 year)
Compression: Gzip + Brotli
HTTP/2 Push: Critical resources
```

## 🔧 Troubleshooting

### Common Issues

1. **Build Failures**
   ```bash
   # Check build logs in Cloudflare dashboard
   # Verify file paths are correct
   # Ensure _headers and _redirects are in root
   ```

2. **Custom Domain Issues**
   ```bash
   # Verify DNS records with dig
   dig articdbm.penguintech.io CNAME
   
   # Check SSL certificate status
   # Wait up to 24 hours for propagation
   ```

3. **404 Errors**
   ```bash
   # Check file paths in repository
   # Verify build output directory
   # Review _redirects file syntax
   ```

### Debug Commands

```bash
# Test DNS resolution
nslookup articdbm.penguintech.io

# Check SSL certificate
openssl s_client -servername articdbm.penguintech.io -connect articdbm.penguintech.io:443

# Test redirects
curl -I https://articdbm.penguintech.io/docs/
```

## 📊 Monitoring & Maintenance

### Regular Checks

- **SSL certificate** expiration (auto-renewed)
- **DNS record** accuracy
- **Performance metrics** via Cloudflare Analytics
- **Security threats** in Firewall tab
- **Build status** in Pages dashboard

### Update Workflow

1. **Content Updates**
   - Edit files in repository
   - Commit to main branch
   - Automatic deployment to production

2. **Configuration Changes**
   - Update `_headers` or `_redirects`
   - Modify DNS records in Cloudflare
   - Test changes thoroughly

3. **Domain Changes**
   - Update custom domain settings
   - Modify DNS records accordingly
   - Update hardcoded URLs in content

---

## 🎯 Quick Setup Checklist

- [ ] Create Cloudflare Pages project
- [ ] Connect GitHub repository  
- [ ] Configure build settings (`/website` directory)
- [ ] Set up custom domain (`articdbm.penguintech.io`)
- [ ] Configure DNS records (CNAME)
- [ ] Enable security features (HTTPS, HSTS)
- [ ] Test website functionality
- [ ] Set up documentation site (optional)
- [ ] Enable Web Analytics
- [ ] Configure firewall rules
- [ ] Set up monitoring alerts

**Deployment Time**: ~15 minutes
**DNS Propagation**: Up to 24 hours  
**SSL Certificate**: Auto-provisioned in minutes

---

*For additional help, check the [Cloudflare Pages documentation](https://developers.cloudflare.com/pages/) or contact support.*