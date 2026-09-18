import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Standard Vite + React setup. The backend API base URL is read from
// VITE_API_BASE_URL at build/dev time (see src/api.js) so the same build
// can point at different backend deployments without code changes.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: true,
  },
})
