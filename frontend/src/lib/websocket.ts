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

function createMarketStore(): Readable<OrderBookSnapshot> {
	return readable<OrderBookSnapshot>(emptySnapshot, (set) => {
		if (!browser) {
			return () => {};
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