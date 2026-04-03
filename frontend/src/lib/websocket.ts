import { browser } from '$app/environment';
import { readable, type Readable } from 'svelte/store';

export type Order = {
	id: string;
	type: 'buy' | 'sell';
	price: number;
	quantity: number;
	createdAt: string;
};

export type OrderBookSnapshot = {
	bids: Order[];
	asks: Order[];
	timestamp: string;
};

const emptySnapshot: OrderBookSnapshot = {
	bids: [],
	asks: [],
	timestamp: new Date(0).toISOString()
};

function createSyntheticStream(set: (snapshot: OrderBookSnapshot) => void): () => void {
	type MutableLevel = {
		price: number;
		quantity: number;
	};

	const levelCount = 10;
	let basePrice = 100;
	let tick = 0;

	const bids: MutableLevel[] = Array.from({ length: levelCount }, (_, index) => ({
		price: basePrice - 0.08 - index * 0.09,
		quantity: Math.round(300 + Math.random() * 900)
	}));

	const asks: MutableLevel[] = Array.from({ length: levelCount }, (_, index) => ({
		price: basePrice + 0.08 + index * 0.09,
		quantity: Math.round(300 + Math.random() * 900)
	}));

	const toOrder = (side: 'buy' | 'sell', level: MutableLevel, index: number): Order => ({
		id: `${side}-${index}-${tick}`,
		type: side,
		price: Number(level.price.toFixed(2)),
		quantity: Math.max(1, Math.round(level.quantity)),
		createdAt: new Date().toISOString()
	});

	const publish = () => {
		tick += 1;
		basePrice += (Math.random() - 0.5) * 0.08;

		for (let index = 0; index < levelCount; index += 1) {
			bids[index].price = basePrice - 0.08 - index * 0.09 + (Math.random() - 0.5) * 0.02;
			asks[index].price = basePrice + 0.08 + index * 0.09 + (Math.random() - 0.5) * 0.02;
			bids[index].quantity += (Math.random() - 0.45) * 120;
			asks[index].quantity += (Math.random() - 0.55) * 120;
		}

		set({
			bids: bids
				.slice()
				.sort((left, right) => right.price - left.price)
				.map((level, index) => toOrder('buy', level, index)),
			asks: asks
				.slice()
				.sort((left, right) => left.price - right.price)
				.map((level, index) => toOrder('sell', level, index)),
			timestamp: new Date().toISOString()
		});
	};

	publish();
	const interval = setInterval(publish, 240);

	return () => {
		clearInterval(interval);
	};
}

function createMarketStore(): Readable<OrderBookSnapshot> {
	return readable<OrderBookSnapshot>(emptySnapshot, (set) => {
		if (!browser) {
			return () => {};
		}

		const isGitHubPages = window.location.hostname.endsWith('github.io');
		const forceDemo = new URLSearchParams(window.location.search).has('demo');
		if (isGitHubPages || forceDemo) {
			return createSyntheticStream(set);
		}

		let ws: WebSocket | null = null;
		let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
		let latest = emptySnapshot;
		let rafId = 0;

		const publishFrame = () => {
			set(latest);
			rafId = requestAnimationFrame(publishFrame);
		};

		const connect = () => {
			const socketProtocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
			ws = new WebSocket(`${socketProtocol}://${window.location.host}/ws`);

			ws.onmessage = (event) => {
				try {
					const parsed = JSON.parse(event.data) as OrderBookSnapshot;
					latest = {
						bids: Array.isArray(parsed.bids) ? parsed.bids : [],
						asks: Array.isArray(parsed.asks) ? parsed.asks : [],
						timestamp: parsed.timestamp ?? new Date().toISOString()
					};
				} catch {
					// Ignore malformed payloads while keeping the socket alive.
				}
			};

			ws.onclose = () => {
				if (reconnectTimer === null) {
					reconnectTimer = setTimeout(() => {
						reconnectTimer = null;
						connect();
					}, 1000);
				}
			};

			ws.onerror = () => {
				ws?.close();
			};
		};

		connect();
		rafId = requestAnimationFrame(publishFrame);

		return () => {
			if (rafId) {
				cancelAnimationFrame(rafId);
			}
			if (reconnectTimer !== null) {
				clearTimeout(reconnectTimer);
			}
			ws?.close();
		};
	});
}

export const marketStore = createMarketStore();