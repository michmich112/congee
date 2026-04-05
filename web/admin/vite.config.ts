import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

/** When using `vite dev` alone, proxy /api to the Go admin server (see config admin.port). */
const adminBackend = process.env.VITE_ADMIN_BACKEND ?? 'http://127.0.0.1:3335';

export default defineConfig({
	resolve: {
		alias: {
			// `style-to-object` expects this peer; hoisting under `npm ci` can hide it from Rollup.
			'inline-style-parser': path.resolve(__dirname, 'node_modules/inline-style-parser')
		}
	},
	plugins: [tailwindcss(), sveltekit()],
	server: {
		proxy: {
			'/api': { target: adminBackend, changeOrigin: true }
		}
	}
});
