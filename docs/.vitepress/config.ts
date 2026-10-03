import { defineConfig } from 'vitepress'

// Deployed to GitHub Pages under the repository name.
const base = process.env.DOCS_BASE || '/duckscope/'

export default defineConfig({
  lang: 'en-US',
  title: 'Duckscope',
  description:
    'Local-first LLM tracing UI: one Go binary, embedded React SPA, DuckDB storage, OTLP/HTTP ingestion.',
  base,

  // API examples reference localhost services that are not navigable from
  // the published site.
  ignoreDeadLinks: 'localhostLinks',

  cleanUrls: true,

  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'API', link: '/api/' },
      { text: 'GitHub', link: 'https://github.com/open-ai-sdk/duckscope' },
    ],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting started', link: '/guide/getting-started' },
          { text: 'Ingesting traces', link: '/guide/ingestion' },
          { text: 'Docker & Compose', link: '/guide/docker' },
          { text: 'Development', link: '/guide/development' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'HTTP API', link: '/api/' },
          { text: 'Configuration', link: '/guide/configuration' },
        ],
      },
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/open-ai-sdk/duckscope' }],
    editLink: {
      pattern: 'https://github.com/open-ai-sdk/duckscope/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
    search: { provider: 'local' },
    footer: {
      message: 'Local development tool — no authentication, trusted networks only.',
    },
  },
})
