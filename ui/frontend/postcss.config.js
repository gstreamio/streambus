// Tailwind and Autoprefixer run through PostCSS.
//
// This file was missing, so neither ran: `@tailwind` and `@apply` reached the
// bundler unprocessed and the shipped stylesheet was under a kilobyte of raw
// at-rules. Vite 5's minifier ignored the unknown at-rules silently; Vite 8's
// (lightningcss) reports them, which is how the gap surfaced.
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}
