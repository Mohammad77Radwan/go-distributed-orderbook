<script lang="ts">
	import { marketStore, type Order } from '$lib/websocket';
	import { fromStore } from 'svelte/store';

	const market = fromStore(marketStore);
	const emptyTimestamp = new Date(0).toISOString();
	let visibleLevels = $state(8);
	let motionPaused = $state(false);
	let focusSide = $state<'overview' | 'bids' | 'asks'>('overview');

	const priceFormatter = new Intl.NumberFormat('en-US', {
		minimumFractionDigits: 2,
		maximumFractionDigits: 2
	});

	const quantityFormatter = new Intl.NumberFormat('en-US', {
		maximumFractionDigits: 0
	});

	const timestampFormatter = new Intl.DateTimeFormat('en-US', {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hour12: false
	});

	const formatPrice = (price: number) => priceFormatter.format(price);
	const formatQuantity = (quantity: number) => quantityFormatter.format(quantity);

	type DepthChartPoint = {
		x: number;
		y: number;
		price: number;
		depth: number;
	};

	type DepthChart = {
		bidLine: string;
		bidArea: string;
		askLine: string;
		askArea: string;
		bidPoints: DepthChartPoint[];
		askPoints: DepthChartPoint[];
		ticks: number[];
		minPrice: number;
		maxPrice: number;
		maxDepth: number;
	};

	type TapeEntry = {
		label: string;
		value: string;
		note: string;
		tone: 'positive' | 'negative' | 'neutral';
	};

	type SnapshotSummary = {
		bidPrice: number | null;
		askPrice: number | null;
		spread: number | null;
		midPrice: number | null;
		bidQuantity: number;
		askQuantity: number;
		imbalance: number;
	};

	const bids = $derived([...market.current.bids].sort((left, right) => right.price - left.price));
	const asks = $derived([...market.current.asks].sort((left, right) => left.price - right.price));
	const topBids = $derived(bids.slice(0, visibleLevels));
	const topAsks = $derived(asks.slice(0, visibleLevels));
	const bestBid = $derived(bids[0]);
	const bestAsk = $derived(asks[0]);
	const spread = $derived(bestBid && bestAsk ? Number((bestAsk.price - bestBid.price).toFixed(2)) : null);
	const midPrice = $derived(bestBid && bestAsk ? Number(((bestAsk.price + bestBid.price) / 2).toFixed(2)) : null);
	const totalBidQuantity = $derived(bids.reduce((sum, order) => sum + order.quantity, 0));
	const totalAskQuantity = $derived(asks.reduce((sum, order) => sum + order.quantity, 0));
	const imbalance = $derived(
		totalBidQuantity + totalAskQuantity === 0
			? 0
			: (totalBidQuantity - totalAskQuantity) / (totalBidQuantity + totalAskQuantity)
	);
	const maxQuantity = $derived(Math.max(1, ...topBids.map((order) => order.quantity), ...topAsks.map((order) => order.quantity)));
	const depthChart = $derived(buildDepthChart(topBids, topAsks));
	const snapshotSummary = $derived({
		bidPrice: bestBid?.price ?? null,
		askPrice: bestAsk?.price ?? null,
		spread,
		midPrice,
		bidQuantity: totalBidQuantity,
		askQuantity: totalAskQuantity,
		imbalance
	});
	const isBooting = $derived(market.current.timestamp === emptyTimestamp);
	const feedState = $derived(isBooting ? 'BOOTING' : 'LIVE');
	const lastUpdated = $derived(
		isBooting ? 'Awaiting first snapshot' : `Updated ${timestampFormatter.format(new Date(market.current.timestamp))}`
	);
	let tape = $state<TapeEntry[]>([]);
	let previousSummary = $state<SnapshotSummary | null>(null);
	const tapeLoop = $derived(tape.length > 0 ? [...tape, ...tape, ...tape] : []);
	const emphasisText = $derived(
		focusSide === 'overview'
			? 'Balanced view'
			: focusSide === 'bids'
				? 'Bid-side spotlight'
				: 'Ask-side spotlight'
	);

	const depthWidth = (quantity: number) => Math.max(8, Math.round((quantity / maxQuantity) * 100));
	const priceDelta = (price: number, reference: number | undefined) => {
		if (!reference) return '—';
		const delta = price - reference;
		return `${delta > 0 ? '+' : ''}${formatPrice(delta)}`;
	};

	function setLevelPreset(levels: number) {
		visibleLevels = levels;
	}

	function toggleMotion() {
		motionPaused = !motionPaused;
	}

	function setFocus(side: 'overview' | 'bids' | 'asks') {
		focusSide = side;
	}

	function buildDepthChart(bids: Order[], asks: Order[]): DepthChart {
		const chartWidth = 1000;
		const chartHeight = 360;
		const chartPadding = 40;
		const innerWidth = chartWidth - chartPadding * 2;
		const innerHeight = chartHeight - chartPadding * 2;

		const makeSeries = (levels: Order[]) => {
			let cumulativeDepth = 0;
			return levels.map((order) => {
				cumulativeDepth += order.quantity;
				return { price: order.price, depth: cumulativeDepth, x: 0, y: 0 };
			});
		};

		const bidSeries = makeSeries(bids);
		const askSeries = makeSeries(asks);
		const priceValues = [...bidSeries, ...askSeries].map((point) => point.price);
		const minPrice = priceValues.length > 0 ? Math.min(...priceValues) : 0;
		const maxPrice = priceValues.length > 0 ? Math.max(...priceValues) : 1;
		const maxDepth = Math.max(1, ...bidSeries.map((point) => point.depth), ...askSeries.map((point) => point.depth));
		const priceSpan = Math.max(0.01, maxPrice - minPrice);
		const baselineY = chartPadding + innerHeight;

		const scaleX = (price: number) => chartPadding + ((price - minPrice) / priceSpan) * innerWidth;
		const scaleY = (depth: number) => chartPadding + innerHeight - (depth / maxDepth) * innerHeight;
		const positionSeries = (series: DepthChartPoint[]) =>
			series.map((point) => ({
				...point,
				x: scaleX(point.price),
				y: scaleY(point.depth)
			}));

		const bidPoints = positionSeries(bidSeries);
		const askPoints = positionSeries(askSeries);

		const buildLine = (points: DepthChartPoint[]) =>
			points.length === 0 ? '' : `M ${points[0].x.toFixed(2)} ${points[0].y.toFixed(2)} ${points.slice(1).map((point) => `L ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ')}`;

		const buildArea = (points: DepthChartPoint[]) => {
			if (points.length === 0) {
				return '';
			}

			const start = points[0];
			const end = points[points.length - 1];
			const lineSegments = points.slice(1).map((point) => `L ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ');
			return `M ${start.x.toFixed(2)} ${baselineY.toFixed(2)} L ${start.x.toFixed(2)} ${start.y.toFixed(2)} ${lineSegments} L ${end.x.toFixed(2)} ${baselineY.toFixed(2)} Z`;
		};

		const ticks = Array.from({ length: 5 }, (_, index) => minPrice + (priceSpan / 4) * index);

		return {
			bidLine: buildLine(bidPoints),
			bidArea: buildArea(bidPoints),
			askLine: buildLine(askPoints),
			askArea: buildArea(askPoints),
			bidPoints,
			askPoints,
			ticks,
			minPrice,
			maxPrice,
			maxDepth
		};
	}

	$effect(() => {
		const currentSummary = snapshotSummary;
		if (!previousSummary) {
			previousSummary = currentSummary;
			return;
		}

		const entries: TapeEntry[] = [];

		if (currentSummary.bidPrice !== previousSummary.bidPrice && currentSummary.bidPrice !== null) {
			entries.push({
				label: 'Bid repriced',
				value: formatPrice(currentSummary.bidPrice),
				note:
					previousSummary.bidPrice === null
						? 'fresh liquidity'
						: priceDelta(currentSummary.bidPrice, previousSummary.bidPrice),
				tone:
					previousSummary.bidPrice === null || currentSummary.bidPrice >= previousSummary.bidPrice
						? 'positive'
						: 'negative'
			});
		}

		if (currentSummary.askPrice !== previousSummary.askPrice && currentSummary.askPrice !== null) {
			entries.push({
				label: 'Ask repriced',
				value: formatPrice(currentSummary.askPrice),
				note:
					previousSummary.askPrice === null
						? 'fresh liquidity'
						: priceDelta(currentSummary.askPrice, previousSummary.askPrice),
				tone:
					previousSummary.askPrice === null || currentSummary.askPrice <= previousSummary.askPrice
						? 'positive'
						: 'negative'
			});
		}

		if (currentSummary.spread !== previousSummary.spread && currentSummary.spread !== null) {
			entries.push({
				label: 'Spread',
				value: formatPrice(currentSummary.spread),
				note:
					previousSummary.spread === null
						? 'market opened'
						: currentSummary.spread < previousSummary.spread
							? 'compressed'
							: 'widened',
				tone:
					previousSummary.spread === null
						? 'neutral'
						: currentSummary.spread < previousSummary.spread
							? 'positive'
							: 'negative'
			});
		}

		const imbalanceShift = (currentSummary.imbalance - previousSummary.imbalance) * 100;
		if (Math.abs(imbalanceShift) >= 2) {
			entries.push({
				label: 'Flow shift',
				value: `${Math.abs(currentSummary.imbalance * 100).toFixed(1)}%`,
				note: currentSummary.imbalance > 0 ? 'bid-led' : currentSummary.imbalance < 0 ? 'ask-led' : 'balanced',
				tone: currentSummary.imbalance > 0 ? 'positive' : currentSummary.imbalance < 0 ? 'negative' : 'neutral'
			});
		}

		if (entries.length > 0) {
			tape = [...entries, ...tape].slice(0, 12);
		}

		previousSummary = currentSummary;
	});
</script>

<svelte:head>
	<title>Quantum Order Book | Live Liquidity Dashboard</title>
	<meta
		name="description"
		content="A cinematic live order book experience with spread intelligence, depth visualization, and real-time market telemetry."
	/>
	<meta name="theme-color" content="#060816" />
</svelte:head>

<main class="dashboard-shell" data-focus={focusSide} class:motion-paused={motionPaused}>
	<section class="hero-card glass-card">
		<div class="hero-copy">
			<p class="eyebrow">Distributed matching engine</p>
			<h1>Quantum Order Book</h1>
			<p class="lede">
				A real-time market cockpit for portfolio viewers who want to see liquidity, spread, and depth
				in one glance.
			</p>
			<div class="status-row">
				<span class="status-pill" data-state={feedState.toLowerCase()}>{feedState}</span>
				<span class="status-note">{lastUpdated}</span>
			</div>

			<div class="control-row" aria-label="Demo controls">
				<button type="button" class="control-button" class:active={focusSide === 'overview'} onclick={() => setFocus('overview')}>
					Overview
				</button>
				<button type="button" class="control-button" class:active={focusSide === 'bids'} onclick={() => setFocus('bids')}>
					Bids
				</button>
				<button type="button" class="control-button" class:active={focusSide === 'asks'} onclick={() => setFocus('asks')}>
					Asks
				</button>
				<button type="button" class="control-button" class:active={motionPaused} onclick={toggleMotion}>
					{motionPaused ? 'Resume motion' : 'Freeze motion'}
				</button>
				<div class="control-group">
					<span>Depth</span>
					<button type="button" class="control-button slim" class:active={visibleLevels === 4} onclick={() => setLevelPreset(4)}>4</button>
					<button type="button" class="control-button slim" class:active={visibleLevels === 8} onclick={() => setLevelPreset(8)}>8</button>
					<button type="button" class="control-button slim" class:active={visibleLevels === 12} onclick={() => setLevelPreset(12)}>12</button>
				</div>
			</div>
		</div>

		<div class="hero-metrics">
			<div class="metric-card">
				<span>Best bid</span>
				<strong>{bestBid ? formatPrice(bestBid.price) : '—'}</strong>
				<small>{bestBid ? formatQuantity(bestBid.quantity) : 'No liquidity'}</small>
			</div>
			<div class="metric-card metric-accent">
				<span>Spread</span>
				<strong>{spread === null ? '—' : formatPrice(spread)}</strong>
				<small>{midPrice === null ? 'Mid unavailable' : `Mid ${formatPrice(midPrice)}`}</small>
			</div>
			<div class="metric-card">
				<span>Best ask</span>
				<strong>{bestAsk ? formatPrice(bestAsk.price) : '—'}</strong>
				<small>{bestAsk ? formatQuantity(bestAsk.quantity) : 'No liquidity'}</small>
			</div>
			<div class="metric-card">
				<span>Imbalance</span>
				<strong>{(imbalance * 100).toFixed(1)}%</strong>
				<small>{imbalance > 0 ? 'Bid-heavy' : imbalance < 0 ? 'Ask-heavy' : 'Balanced'}</small>
			</div>
			<div class="metric-card metric-wide">
				<span>Presentation mode</span>
				<strong>{emphasisText}</strong>
				<small>{motionPaused ? 'Motion paused for narration' : 'Live motion active'}</small>
			</div>
		</div>
	</section>

	<section class="stats-row">
		<article class="stat-card glass-card">
			<span>Total bid liquidity</span>
			<strong>{formatQuantity(totalBidQuantity)}</strong>
			<small>{topBids.length} visible levels</small>
		</article>
		<article class="stat-card glass-card">
			<span>Total ask liquidity</span>
			<strong>{formatQuantity(totalAskQuantity)}</strong>
			<small>{topAsks.length} visible levels</small>
		</article>
		<article class="stat-card glass-card">
			<span>Reference price</span>
			<strong>{midPrice === null ? '—' : formatPrice(midPrice)}</strong>
			<small>{spread === null ? 'No spread yet' : `Spread ${formatPrice(spread)}`}</small>
		</article>
		<article class="stat-card glass-card">
			<span>Feed state</span>
			<strong>{feedState}</strong>
			<small>{isBooting ? 'Connecting to stream' : 'Snapshots flowing'}</small>
		</article>
	</section>

	<section class="chart-card glass-card">
		<div class="panel-head">
			<div>
				<p class="panel-kicker">Market geometry</p>
				<h2>Animated depth curve</h2>
			</div>
			<div class="panel-badge">Live snapshot</div>
		</div>

		<div class="chart-shell">
			<div class="chart-legend">
				<div>
					<span class="legend-dot legend-dot-bid"></span>
					<strong>Bids</strong>
					<small>Accumulating liquidity</small>
				</div>
				<div>
					<span class="legend-dot legend-dot-ask"></span>
					<strong>Asks</strong>
					<small>Pressure above mid</small>
				</div>
				<div>
					<span class="legend-dot legend-dot-mid"></span>
					<strong>Mid</strong>
					<small>{midPrice === null ? 'Unavailable' : formatPrice(midPrice)}</small>
				</div>
			</div>

			<svg class="depth-chart" viewBox="0 0 1000 360" role="img" aria-label="Depth chart">
					<defs>
						<linearGradient id="bidFill" x1="0" y1="0" x2="0" y2="1">
							<stop offset="0%" stop-color="rgba(124, 241, 164, 0.42)" />
							<stop offset="100%" stop-color="rgba(124, 241, 164, 0.04)" />
						</linearGradient>
						<linearGradient id="askFill" x1="0" y1="0" x2="0" y2="1">
							<stop offset="0%" stop-color="rgba(255, 139, 139, 0.42)" />
							<stop offset="100%" stop-color="rgba(255, 139, 139, 0.04)" />
						</linearGradient>
						<linearGradient id="gridFade" x1="0" y1="0" x2="1" y2="0">
							<stop offset="0%" stop-color="rgba(255,255,255,0.02)" />
							<stop offset="50%" stop-color="rgba(255,255,255,0.12)" />
							<stop offset="100%" stop-color="rgba(255,255,255,0.02)" />
						</linearGradient>
					</defs>

					{#each depthChart.ticks as tick, index}
						<g>
							<line
								x1={40 + (920 / 4) * index}
								y1="40"
								x2={40 + (920 / 4) * index}
								y2="320"
								stroke="url(#gridFade)"
								stroke-width="1"
							/>
							<text x={40 + (920 / 4) * index} y="344" text-anchor="middle">{formatPrice(tick)}</text>
						</g>
					{/each}

					<line x1="40" y1="200" x2="960" y2="200" class="mid-line" />
					<path d={depthChart.bidArea} class="chart-area chart-area-bid" />
					<path d={depthChart.askArea} class="chart-area chart-area-ask" />
					<path d={depthChart.bidLine} class="chart-line chart-line-bid" />
					<path d={depthChart.askLine} class="chart-line chart-line-ask" />

					{#if bestBid}
						<circle
							cx={Math.max(40, Math.min(960, 40 + ((bestBid.price - depthChart.minPrice) / Math.max(0.01, depthChart.maxPrice - depthChart.minPrice)) * 920))}
							cy={40 + 280 - (bestBid.quantity / Math.max(1, depthChart.maxDepth)) * 280}
							r="5"
							class="chart-point chart-point-bid"
						/>
					{/if}

					{#if bestAsk}
						<circle
							cx={Math.max(40, Math.min(960, 40 + ((bestAsk.price - depthChart.minPrice) / Math.max(0.01, depthChart.maxPrice - depthChart.minPrice)) * 920))}
							cy={40 + 280 - (bestAsk.quantity / Math.max(1, depthChart.maxDepth)) * 280}
							r="5"
							class="chart-point chart-point-ask"
						/>
					{/if}
				</svg>
		</div>
	</section>

	<section class="tape-card glass-card">
		<div class="panel-head">
			<div>
				<p class="panel-kicker">Trade tape</p>
				<h2>Live market reactions</h2>
			</div>
			<div class="panel-badge">Newest first</div>
		</div>

		<div class="tape-rail">
			<div class="tape-track">
				{#if tapeLoop.length === 0}
					<div class="tape-chip tape-chip-empty">Waiting for the first market reaction...</div>
				{:else}
					{#each tapeLoop as item}
						<div class="tape-chip tone-{item.tone}">
							<span>{item.label}</span>
							<strong>{item.value}</strong>
							<small>{item.note}</small>
						</div>
					{/each}
				{/if}
			</div>
		</div>
	</section>

	<section class="workspace-grid">
		<article class="panel glass-card depth-panel">
			<div class="panel-head">
				<div>
					<p class="panel-kicker">Liquidity map</p>
					<h2>Depth of book</h2>
				</div>
				<div class="panel-badge">Top {visibleLevels} levels</div>
			</div>

			<div class="depth-columns">
				<div class="depth-column depth-column-bids">
					<div class="column-label">Bids</div>
					{#if topBids.length === 0}
						<p class="empty-state">Waiting for bids...</p>
					{:else}
						{#each topBids as bid, index}
							<div
								class="depth-row depth-row-bid"
								class:best={index === 0}
								style={`--row-delay:${index * 70}ms; --fill:${depthWidth(bid.quantity)}%;`}
							>
								<div class="row-bar row-bar-bid"></div>
								<div class="row-copy">
									<div>
										<span class="row-price bid-price">{formatPrice(bid.price)}</span>
										<small>{priceDelta(bid.price, bestBid?.price)}</small>
									</div>
									<strong>{formatQuantity(bid.quantity)}</strong>
								</div>
							</div>
						{/each}
					{/if}
				</div>

				<div class="depth-divider">
					<span>Spread</span>
					<strong>{spread === null ? '—' : formatPrice(spread)}</strong>
				</div>

				<div class="depth-column depth-column-asks">
					<div class="column-label">Asks</div>
					{#if topAsks.length === 0}
						<p class="empty-state">Waiting for asks...</p>
					{:else}
						{#each topAsks as ask, index}
							<div
								class="depth-row depth-row-ask"
								class:best={index === 0}
								style={`--row-delay:${index * 70}ms; --fill:${depthWidth(ask.quantity)}%;`}
							>
								<div class="row-bar row-bar-ask"></div>
								<div class="row-copy">
									<div>
										<span class="row-price ask-price">{formatPrice(ask.price)}</span>
										<small>{priceDelta(ask.price, bestAsk?.price)}</small>
									</div>
									<strong>{formatQuantity(ask.quantity)}</strong>
								</div>
							</div>
						{/each}
					{/if}
				</div>
			</div>
		</article>

		<article class="panel glass-card book-panel">
			<div class="panel-head">
				<div>
					<p class="panel-kicker">Order ladder</p>
					<h2>Best levels in the book</h2>
				</div>
				<div class="panel-badge">Bid / Ask</div>
			</div>

			<div class="book-grid">
				<div class="book-side">
					<div class="side-head side-head-bid">
						<span>Bid price</span>
						<span>Size</span>
					</div>
					{#if topBids.length === 0}
						<p class="empty-state compact">No bid-side orders yet.</p>
					{:else}
						{#each topBids as bid, index}
							<div class="book-row" class:highlight={index === 0}>
								<div>
									<span class="row-price bid-price">{formatPrice(bid.price)}</span>
									<small>{index === 0 ? 'Best bid' : `Level ${index + 1}`}</small>
								</div>
								<strong>{formatQuantity(bid.quantity)}</strong>
							</div>
						{/each}
					{/if}
				</div>

				<div class="book-side">
					<div class="side-head side-head-ask">
						<span>Ask price</span>
						<span>Size</span>
					</div>
					{#if topAsks.length === 0}
						<p class="empty-state compact">No ask-side orders yet.</p>
					{:else}
						{#each topAsks as ask, index}
							<div class="book-row" class:highlight={index === 0}>
								<div>
									<span class="row-price ask-price">{formatPrice(ask.price)}</span>
									<small>{index === 0 ? 'Best ask' : `Level ${index + 1}`}</small>
								</div>
								<strong>{formatQuantity(ask.quantity)}</strong>
							</div>
						{/each}
					{/if}
				</div>
			</div>
		</article>
	</section>
</main>

<style>
	:global(body) {
		margin: 0;
		min-height: 100vh;
		background:
			radial-gradient(circle at top left, rgba(88, 129, 255, 0.18), transparent 28%),
			radial-gradient(circle at top right, rgba(255, 147, 77, 0.16), transparent 26%),
			linear-gradient(135deg, #050816 0%, #09111f 45%, #04070d 100%);
		font-family:
			var(--font-mono);
		color: #edf2ff;
		text-rendering: optimizeLegibility;
		-webkit-font-smoothing: antialiased;
		-moz-osx-font-smoothing: grayscale;
	}

	:global(body::before) {
		content: '';
		position: fixed;
		inset: 0;
		pointer-events: none;
		background-image:
			radial-gradient(rgba(255, 255, 255, 0.035) 1px, transparent 1px),
			linear-gradient(rgba(255, 255, 255, 0.03) 1px, transparent 1px);
		background-size: 34px 34px, 100% 100%;
		mask-image: radial-gradient(circle at center, black 48%, transparent 100%);
		opacity: 0.45;
	}

	:global(::selection) {
		background: rgba(120, 170, 255, 0.35);
		color: #ffffff;
	}

	.dashboard-shell {
		width: min(1360px, calc(100vw - 2rem));
		margin: 0 auto;
		padding: 2rem 0 3rem;
		position: relative;
		z-index: 1;
	}

	.glass-card {
		border: 1px solid rgba(180, 200, 255, 0.16);
		background: linear-gradient(180deg, rgba(8, 13, 28, 0.88), rgba(5, 8, 19, 0.75));
		backdrop-filter: blur(18px);
		box-shadow:
			0 32px 70px rgba(0, 0, 0, 0.45),
			inset 0 1px 0 rgba(255, 255, 255, 0.04);
	}

	.hero-card {
		display: grid;
		grid-template-columns: minmax(0, 1.4fr) minmax(320px, 0.95fr);
		gap: 1.5rem;
		padding: 1.5rem;
		border-radius: 28px;
	}

	.hero-copy {
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		gap: 1.25rem;
	}

	.eyebrow,
	.panel-kicker,
	.column-label,
	.side-head span,
	.stat-card span,
	.metric-card span,
	.status-note {
		text-transform: uppercase;
		letter-spacing: 0.18em;
		font-size: 0.72rem;
		color: rgba(201, 215, 255, 0.68);
	}

	h1 {
		margin: 0;
		font-size: clamp(2.35rem, 6vw, 4.8rem);
		line-height: 0.94;
		letter-spacing: -0.06em;
		max-width: 10ch;
		font-family: var(--font-display);
	}

	.lede {
		margin: 0;
		max-width: 60ch;
		font-size: 1.02rem;
		line-height: 1.75;
		color: rgba(228, 236, 255, 0.78);
	}

	.status-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: center;
	}

	.status-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.65rem 0.9rem;
		border-radius: 999px;
		font-size: 0.75rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		border: 1px solid rgba(255, 255, 255, 0.12);
		background: rgba(255, 255, 255, 0.04);
	}

	.status-pill::before {
		content: '';
		width: 0.55rem;
		height: 0.55rem;
		border-radius: 999px;
		background: #ffbf6b;
		box-shadow: 0 0 0 6px rgba(255, 191, 107, 0.12);
	}

	.status-pill[data-state='live']::before {
		background: #57e58d;
		box-shadow: 0 0 0 6px rgba(87, 229, 141, 0.14);
	}

	.status-note {
		font-size: 0.78rem;
		color: rgba(230, 237, 255, 0.72);
	}

	.control-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		padding-top: 0.25rem;
	}

	.control-group {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		padding: 0.2rem 0.35rem 0.2rem 0.7rem;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(180, 200, 255, 0.1);
		font-size: 0.72rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: rgba(201, 215, 255, 0.72);
	}

	.control-button {
		appearance: none;
		border: 1px solid rgba(180, 200, 255, 0.14);
		background: rgba(255, 255, 255, 0.04);
		color: #edf2ff;
		border-radius: 999px;
		padding: 0.72rem 1rem;
		font: inherit;
		font-size: 0.8rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		cursor: pointer;
		transition:
			transform 140ms ease,
			border-color 140ms ease,
			background 140ms ease,
			box-shadow 140ms ease;
	}

	.control-button:hover {
		transform: translateY(-1px);
		border-color: rgba(123, 226, 255, 0.32);
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
	}

	.control-button.active {
		background: rgba(123, 226, 255, 0.12);
		border-color: rgba(123, 226, 255, 0.36);
		color: #b8f0ff;
	}

	.control-button.slim {
		padding: 0.45rem 0.62rem;
		font-size: 0.72rem;
	}

	.hero-metrics {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.85rem;
	}

	.metric-card {
		padding: 1rem;
		border-radius: 20px;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.055), rgba(255, 255, 255, 0.02));
		border: 1px solid rgba(180, 200, 255, 0.12);
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
		min-height: 118px;
	}

	.metric-card strong,
	.stat-card strong,
	.panel-badge,
	.depth-divider strong,
	.book-row strong {
		font-variant-numeric: tabular-nums;
	}

	.metric-card strong {
		font-size: clamp(1.1rem, 2vw, 1.6rem);
		letter-spacing: -0.04em;
		font-family: var(--font-mono);
	}

	.metric-wide {
		grid-column: 1 / -1;
		min-height: auto;
	}

	.metric-card small,
	.stat-card small,
	.row-copy small {
		color: rgba(224, 231, 248, 0.66);
		font-size: 0.78rem;
	}

	.metric-accent {
		background:
			radial-gradient(circle at top right, rgba(77, 175, 255, 0.2), transparent 55%),
			linear-gradient(180deg, rgba(255, 255, 255, 0.08), rgba(255, 255, 255, 0.025));
	}

	.stats-row {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 0.9rem;
		margin-top: 0.9rem;
	}

	.stat-card {
		padding: 1rem 1.1rem;
		border-radius: 20px;
		display: flex;
		flex-direction: column;
		gap: 0.45rem;
	}

	.stat-card strong {
		font-size: 1.2rem;
		font-family: var(--font-mono);
	}

	.workspace-grid {
		display: grid;
		grid-template-columns: minmax(0, 1.15fr) minmax(320px, 0.85fr);
		gap: 0.95rem;
		margin-top: 0.95rem;
	}

	.panel {
		border-radius: 26px;
		padding: 1.1rem;
	}

	.chart-card,
	.tape-card {
		border-radius: 26px;
		padding: 1.1rem;
		margin-top: 0.95rem;
	}

	.panel-head {
		display: flex;
		justify-content: space-between;
		align-items: start;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.panel-head h2 {
		margin: 0.3rem 0 0;
		font-size: 1.25rem;
		letter-spacing: -0.03em;
		font-family: var(--font-display);
	}

	.panel-badge {
		padding: 0.55rem 0.8rem;
		border-radius: 999px;
		font-size: 0.72rem;
		border: 1px solid rgba(180, 200, 255, 0.16);
		background: rgba(255, 255, 255, 0.04);
		color: rgba(233, 239, 255, 0.82);
	}

	.depth-columns {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
		gap: 0.9rem;
		align-items: stretch;
	}

	.depth-column {
		display: flex;
		flex-direction: column;
		gap: 0.58rem;
	}

	.depth-column-asks {
		text-align: right;
	}

	.depth-divider {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.35rem;
		padding: 1rem 0.65rem;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.035);
		border: 1px solid rgba(180, 200, 255, 0.12);
		min-width: 88px;
		align-self: center;
	}

	.depth-divider span {
		font-size: 0.7rem;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: rgba(205, 216, 245, 0.65);
	}

	.depth-divider strong {
		font-size: 1rem;
	}

	.depth-row {
		position: relative;
		overflow: hidden;
		border-radius: 16px;
		padding: 0.9rem 0.95rem;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(180, 200, 255, 0.11);
		animation: rise 420ms ease both;
		animation-delay: var(--row-delay);
	}

	.depth-row.best {
		border-color: rgba(255, 255, 255, 0.23);
		background: rgba(255, 255, 255, 0.06);
	}

	.row-bar {
		position: absolute;
		inset: 0;
		width: var(--fill);
		opacity: 0.9;
	}

	.row-bar-bid {
		left: 0;
		background: linear-gradient(90deg, rgba(56, 211, 128, 0.28), rgba(56, 211, 128, 0.04));
	}

	.row-bar-ask {
		right: 0;
		margin-left: auto;
		background: linear-gradient(270deg, rgba(255, 104, 104, 0.28), rgba(255, 104, 104, 0.04));
	}

	.row-copy {
		position: relative;
		z-index: 1;
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.75rem;
	}

	.row-copy div {
		display: flex;
		flex-direction: column;
		gap: 0.14rem;
	}

	.row-price {
		font-size: 0.98rem;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
		font-family: var(--font-mono);
	}

	.bid-price {
		color: #7cf1a4;
	}

	.ask-price {
		color: #ff8b8b;
	}

	.row-copy strong {
		font-size: 0.98rem;
	}

	.empty-state {
		margin: 0;
		padding: 1rem;
		border-radius: 16px;
		border: 1px dashed rgba(180, 200, 255, 0.16);
		color: rgba(228, 235, 248, 0.68);
		background: rgba(255, 255, 255, 0.02);
	}

	.empty-state.compact {
		padding: 0.9rem;
	}

	.book-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.85rem;
	}

	.book-side {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
	}

	.side-head {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0 0.35rem;
	}

	.side-head span {
		font-size: 0.68rem;
	}

	.side-head-bid span:last-child {
		color: rgba(124, 241, 164, 0.72);
	}

	.side-head-ask span:last-child {
		color: rgba(255, 139, 139, 0.72);
	}

	.book-row {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.92rem 0.9rem;
		border-radius: 16px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(180, 200, 255, 0.1);
	}

	.book-row.highlight {
		background: rgba(255, 255, 255, 0.06);
		border-color: rgba(255, 255, 255, 0.18);
	}

	.book-row div {
		display: flex;
		flex-direction: column;
		gap: 0.12rem;
	}

	.book-row small {
		color: rgba(223, 231, 248, 0.66);
		font-size: 0.76rem;
	}

	.depth-panel,
	.book-panel {
		overflow: hidden;
	}

	.chart-shell {
		display: grid;
		gap: 1rem;
	}

	.chart-legend {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.8rem;
	}

	.chart-legend > div {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		padding: 0.8rem 0.9rem;
		border-radius: 18px;
		background: rgba(255, 255, 255, 0.03);
		border: 1px solid rgba(180, 200, 255, 0.1);
	}

	.chart-legend strong {
		display: block;
		font-size: 0.94rem;
	}

	.chart-legend small {
		display: block;
		color: rgba(223, 231, 248, 0.66);
		font-size: 0.73rem;
	}

	.legend-dot {
		width: 0.75rem;
		height: 0.75rem;
		border-radius: 999px;
		flex: 0 0 auto;
	}

	.legend-dot-bid {
		background: var(--accent-emerald);
		box-shadow: 0 0 0 6px rgba(124, 241, 164, 0.12);
	}

	.legend-dot-ask {
		background: var(--accent-rose);
		box-shadow: 0 0 0 6px rgba(255, 139, 139, 0.12);
	}

	.legend-dot-mid {
		background: var(--accent-amber);
		box-shadow: 0 0 0 6px rgba(255, 191, 107, 0.12);
	}

	.depth-chart {
		width: 100%;
		height: auto;
		aspect-ratio: 1000 / 360;
		overflow: visible;
	}

	.depth-chart text {
		fill: rgba(223, 231, 248, 0.52);
		font-size: 18px;
		font-family: var(--font-mono);
		letter-spacing: 0.02em;
	}

	.mid-line {
		stroke: rgba(255, 255, 255, 0.12);
		stroke-dasharray: 8 10;
	}

	.chart-line,
	.chart-area,
	.chart-point {
		vector-effect: non-scaling-stroke;
		animation: chartReveal 1.15s ease both;
	}

	.chart-line {
		fill: none;
		stroke-width: 4;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.chart-line-bid {
		stroke: var(--accent-emerald);
		filter: drop-shadow(0 0 10px rgba(124, 241, 164, 0.24));
	}

	.chart-line-ask {
		stroke: var(--accent-rose);
		filter: drop-shadow(0 0 10px rgba(255, 139, 139, 0.22));
	}

	.chart-area {
		opacity: 0.95;
	}

	.chart-area-bid {
		fill: url(#bidFill);
	}

	.chart-area-ask {
		fill: url(#askFill);
	}

	.chart-point-bid {
		fill: var(--accent-emerald);
		filter: drop-shadow(0 0 8px rgba(124, 241, 164, 0.35));
	}

	.chart-point-ask {
		fill: var(--accent-rose);
		filter: drop-shadow(0 0 8px rgba(255, 139, 139, 0.35));
	}

	.tape-rail {
		overflow: hidden;
		border-radius: 20px;
		border: 1px solid rgba(180, 200, 255, 0.1);
		background: rgba(255, 255, 255, 0.02);
	}

	.tape-track {
		display: flex;
		gap: 0.85rem;
		padding: 0.85rem;
		width: max-content;
		animation: tapeDrift 34s linear infinite;
	}

	.motion-paused .chart-line,
	.motion-paused .chart-area,
	.motion-paused .chart-point,
	.motion-paused .tape-track,
	.motion-paused .depth-row {
		animation-play-state: paused !important;
	}

	.dashboard-shell[data-focus='bids'] .depth-column-bids,
	.dashboard-shell[data-focus='bids'] .book-side:first-child,
	.dashboard-shell[data-focus='asks'] .depth-column-asks,
	.dashboard-shell[data-focus='asks'] .book-side:last-child {
		transform: scale(1.01);
	}

	.dashboard-shell[data-focus='bids'] .depth-column-asks,
	.dashboard-shell[data-focus='bids'] .book-side:last-child,
	.dashboard-shell[data-focus='asks'] .depth-column-bids,
	.dashboard-shell[data-focus='asks'] .book-side:first-child {
		opacity: 0.68;
		filter: saturate(0.85);
	}

	.dashboard-shell[data-focus='overview'] .depth-column,
	.dashboard-shell[data-focus='overview'] .book-side {
		opacity: 1;
		filter: none;
		transform: none;
	}

	.tape-chip {
		display: grid;
		gap: 0.25rem;
		min-width: 170px;
		padding: 0.9rem 1rem;
		border-radius: 16px;
		background: rgba(255, 255, 255, 0.04);
		border: 1px solid rgba(180, 200, 255, 0.12);
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03);
	}

	.tape-chip span {
		text-transform: uppercase;
		font-size: 0.66rem;
		letter-spacing: 0.2em;
		color: rgba(201, 215, 255, 0.64);
	}

	.tape-chip strong {
		font-size: 1rem;
		font-family: var(--font-mono);
		font-variant-numeric: tabular-nums;
	}

	.tape-chip small {
		font-size: 0.74rem;
		color: rgba(223, 231, 248, 0.66);
	}

	.tape-chip-empty {
		min-width: auto;
		padding-inline: 1.1rem;
		color: rgba(223, 231, 248, 0.72);
	}

	.tone-positive {
		border-color: rgba(124, 241, 164, 0.2);
		background: rgba(124, 241, 164, 0.06);
	}

	.tone-negative {
		border-color: rgba(255, 139, 139, 0.2);
		background: rgba(255, 139, 139, 0.06);
	}

	.tone-neutral {
		border-color: rgba(255, 191, 107, 0.2);
		background: rgba(255, 191, 107, 0.06);
	}

	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	@keyframes chartReveal {
		from {
			opacity: 0;
			transform: scaleY(0.97);
			transform-origin: center bottom;
		}
		to {
			opacity: 1;
			transform: scaleY(1);
		}
	}

	@keyframes tapeDrift {
		from {
			transform: translateX(0);
		}
		to {
			transform: translateX(-33.333%);
		}
	}

	@media (max-width: 1080px) {
		.hero-card,
		.workspace-grid {
			grid-template-columns: 1fr;
		}

		.chart-legend {
			grid-template-columns: 1fr;
		}

		.hero-metrics,
		.stats-row,
		.book-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		.depth-columns {
			grid-template-columns: 1fr;
		}

		.depth-divider {
			min-width: 0;
			width: 100%;
			border-radius: 18px;
			padding: 0.9rem;
		}
	}

	@media (max-width: 720px) {
		.dashboard-shell {
			width: min(100vw - 1rem, 1360px);
			padding: 0.5rem 0 2rem;
		}

		.hero-card,
		.panel,
		.stat-card,
		.metric-card {
			border-radius: 20px;
		}

		.hero-metrics,
		.stats-row,
		.book-grid {
			grid-template-columns: 1fr;
		}

		.control-row {
			grid-template-columns: 1fr;
		}

		.tape-track {
			animation-duration: 22s;
		}

		h1 {
			max-width: none;
		}

		.panel-head,
		.row-copy,
		.book-row {
			flex-direction: column;
			align-items: flex-start;
		}

		.depth-row,
		.book-row {
			padding: 0.85rem;
		}
	}
</style>
