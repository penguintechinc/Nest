# ArticDBM Website

This is the static website for ArticDBM, designed to be deployed on Cloudflare Pages.

## 🌐 Live Site

The website is deployed at: **https://articdbm.penguintech.io**

## 🏗️ Development

### Local Development

```bash
# Navigate to website directory
cd website

# Start local server (Python)
python -m http.server 8080

# Or use Node.js serve
npx serve .

# Visit http://localhost:8080
```

### File Structure

```
website/
├── index.html          # Main homepage
├── style.css           # Arctic-themed styles
├── script.js           # Interactive functionality
├── _headers            # Cloudflare Pages security headers
├── _redirects          # URL redirects
├── package.json        # NPM configuration
└── README.md          # This file
```

## 🎨 Design Theme

The website uses an **Arctic theme** with cool colors:

- **Primary Blue**: `#1e3a8a` - Deep arctic blue
- **Ice Blue**: `#dbeafe` - Light icy blue
- **Frost White**: `#f8fafc` - Clean arctic white
- **Snow White**: `#ffffff` - Pure snow
- **Arctic Gray**: `#64748b` - Cool gray tones

## ✨ Features

- **Responsive Design**: Mobile-first approach
- **Smooth Animations**: CSS transitions and JavaScript interactions
- **Performance Optimized**: Minimal dependencies, fast loading
- **Accessibility**: Semantic HTML and ARIA compliance
- **SEO Friendly**: Meta tags and structured data
- **Arctic Theme**: Cool colors and winter-inspired design

## 🚀 Deployment

### Cloudflare Pages

1. **Connect Repository**: Link your GitHub repository to Cloudflare Pages
2. **Build Settings**:
   - Build command: `npm run build` (or leave empty for static)
   - Output directory: `.` (root directory)
3. **Environment Variables**: None required
4. **Custom Domain**: Configure `articdbm.penguintech.io`

### Alternative Deployment

The site can be deployed to any static hosting service:

- **Netlify**: Drag and drop or Git integration
- **Vercel**: Import from GitHub
- **GitHub Pages**: Enable in repository settings
- **AWS S3**: Static website hosting

## 📱 Browser Support

- **Modern Browsers**: Chrome 90+, Firefox 88+, Safari 14+, Edge 90+
- **Mobile**: iOS Safari 14+, Chrome Mobile 90+
- **Features**: CSS Grid, Flexbox, Intersection Observer API

## 🛠️ Customization

### Colors

Update the CSS custom properties in `style.css`:

```css
:root {
    --primary-blue: #1e3a8a;
    --light-blue: #3b82f6;
    --ice-blue: #dbeafe;
    /* ... */
}
```

### Content

Edit `index.html` to update:
- Hero section text
- Feature descriptions
- Statistics
- Footer links

### Interactive Features

Modify `script.js` to add:
- Analytics tracking
- Additional animations
- Custom interactions

## 📊 Performance

- **Page Size**: < 100KB total
- **Load Time**: < 1 second on 3G
- **Core Web Vitals**: Green scores
- **Lighthouse Score**: 95+ across all metrics

## 🔒 Security

Security headers configured in `_headers`:
- Content Security Policy
- X-Frame-Options: DENY
- X-Content-Type-Options: nosniff
- Referrer Policy controls

## 📈 Analytics

To add analytics, modify `script.js` and add your tracking code:

```javascript
// Google Analytics
gtag('config', 'GA_TRACKING_ID');

// Cloudflare Web Analytics
// Add beacon script to index.html
```

## 🎯 SEO Optimizations

- Semantic HTML structure
- Meta description and keywords
- Open Graph tags
- Twitter Card meta
- Structured data (JSON-LD)
- Optimized images and alt text

---

*Keep the arctic theme cool! ❄️*