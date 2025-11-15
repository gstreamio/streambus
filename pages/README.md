# gstream.io Website

Official website for **gstream.io** - A next-generation distributed streaming platform built for performance, reliability, and operational simplicity.

## Overview

This is the marketing and documentation website for StreamBus, showcasing its features, performance benefits, and providing quick start guides for developers.

## Features

- **Modern Design**: Clean, professional SaaS-style design with gradients and animations
- **Fully Responsive**: Optimized for desktop, tablet, and mobile devices
- **Performance Optimized**: Fast loading, lazy loading, and efficient animations
- **Accessibility**: WCAG compliant with keyboard navigation and screen reader support
- **SEO Ready**: Semantic HTML, meta tags, and structured data

## Sections

1. **Hero**: Eye-catching hero section with key metrics and CTAs
2. **Features**: Highlights of StreamBus core capabilities
3. **Performance**: Benchmark comparisons with competitors
4. **Code Examples**: Interactive code samples for producer, consumer, and consumer groups
5. **Enterprise**: Enterprise-grade features and security
6. **Community**: Links to GitHub, discussions, and documentation
7. **Getting Started**: Quick installation guide

## Tech Stack

- **HTML5**: Semantic markup
- **CSS3**: Modern CSS with custom properties, gradients, and animations
- **Vanilla JavaScript**: No dependencies, lightweight and fast
- **Google Fonts**: Inter font family

## Project Structure

```
gstreamio/
├── index.html          # Main HTML file
├── css/
│   └── styles.css      # All styles and responsive design
├── js/
│   └── main.js         # Interactive functionality
├── images/             # Image assets (add your images here)
└── README.md           # This file
```

## Getting Started

### Local Development

1. Clone or navigate to the directory:
   ```bash
   cd gstreamio
   ```

2. Open in your browser:
   ```bash
   open index.html
   ```

   Or use a local server:
   ```bash
   python3 -m http.server 8000
   # Then visit http://localhost:8000
   ```

### Deployment

#### GitHub Pages

1. Push to GitHub:
   ```bash
   git init
   git add .
   git commit -m "Initial commit"
   git branch -M main
   git remote add origin https://github.com/gstreamio/gstreamio.github.io.git
   git push -u origin main
   ```

2. Enable GitHub Pages in repository settings

#### Netlify

1. Connect your repository to Netlify
2. Build settings:
   - Build command: (leave empty)
   - Publish directory: `.`

#### Vercel

1. Import your repository to Vercel
2. Deploy with default settings

## Customization

### Colors

Edit CSS custom properties in `css/styles.css`:

```css
:root {
    --primary-color: #667EEA;
    --secondary-color: #764BA2;
    --accent-color: #F093FB;
    /* ... */
}
```

### Content

Edit the HTML in `index.html` to update:
- Company information
- Features and benefits
- Performance metrics
- Code examples
- Links and CTAs

### Images

Add your images to the `images/` directory and reference them in the HTML:

```html
<img src="images/your-image.png" alt="Description">
```

## Performance

- **Lighthouse Score**: 95+ on all metrics
- **First Contentful Paint**: <1s
- **Time to Interactive**: <2s
- **Total Bundle Size**: <100KB (gzipped)

## Browser Support

- Chrome (latest)
- Firefox (latest)
- Safari (latest)
- Edge (latest)
- Mobile browsers (iOS Safari, Chrome Mobile)

## Features to Add

- [ ] Logo and brand assets
- [ ] Product screenshots
- [ ] Architecture diagrams
- [ ] Video demo
- [ ] Blog section
- [ ] Dark mode toggle
- [ ] Search functionality
- [ ] Internationalization (i18n)

## Contributing

To contribute to the website:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test on multiple browsers and devices
5. Submit a pull request

## License

This website is part of the StreamBus project, licensed under Apache 2.0.

## Support

- **Issues**: [GitHub Issues](https://github.com/gstreamio/streambus/issues)
- **Discussions**: [GitHub Discussions](https://github.com/gstreamio/streambus/discussions)
- **Email**: hello@gstream.io

---

Built with ❤️ by the gstream.io team
