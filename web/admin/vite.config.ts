import process from 'node:process';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

/** When using `vite dev` alone, proxy /api to the Go admin server (see config admin.port). */
const adminBackend = process.env.VITE_ADMIN_BACKEND ?? 'http://127.0.0.1:3335';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			'/api': { target: adminBackend, changeOrigin: true }
		}
	}
});
