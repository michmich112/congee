<script lang="ts">
	import { Button } from '$lib/components/ui/button';
</script>

<main class="mx-auto flex min-h-[70vh] max-w-lg flex-col justify-center gap-6 px-6 py-16">
	<h1 class="text-3xl font-semibold tracking-tight">Congee admin</h1>
	<p class="text-muted-foreground text-sm leading-relaxed">
		SvelteKit shell for the Congee relay control plane. System appearance follows
		<code class="bg-muted rounded px-1 py-0.5 text-xs">prefers-color-scheme</code>
		until auth and API wiring land in later phases.
	</p>
	<div>
		<Button href="https://github.com/nostr-protocol/nips" variant="outline" target="_blank" rel="noreferrer">
			NIPs reference
		</Button>
	</div>
</main>
