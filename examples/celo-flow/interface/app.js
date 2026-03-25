const NATIVE_TOKEN_PLACEHOLDERS = new Set([
  "0x0000000000000000000000000000000000000000",
  "native",
]);

const DEFAULT_TOKEN_IMAGE = "/public/token-default.svg";
const DEFAULT_AMOUNT_IN = "10";

const HERO_TOKEN_ADDRESSES = {
  usdm: "0x765de816845861e75a25fca122bb6898b8b1282a",
  usdc: "0xceba9300f2b948710d2653dd7b07f33a8b32118c",
  usdt: "0x48065fbbe25f71c9282ddf5e1cd6d6a887483d5e",
  dai: "0xac177de2439bd0c7659c61f373dbf247d1f41abe",
  eurm: "0xd8763cba276a3738e6de85b4b3bf5fded6d6ca73",
  btc: "0x8ac2901dd8a1f17a1a4768a6ba4c3751e3995b2d",
  eth: "0xd221812de1bd094f35587ee8e174b07b6167d9af",
};

const STABLE_TOKEN_ADDRESSES = new Set([
  HERO_TOKEN_ADDRESSES.usdm,
  HERO_TOKEN_ADDRESSES.usdc,
  HERO_TOKEN_ADDRESSES.usdt,
  HERO_TOKEN_ADDRESSES.dai,
  HERO_TOKEN_ADDRESSES.eurm,
]);

const ALLOWED_TOKEN_ADDRESSES = new Set([
  HERO_TOKEN_ADDRESSES.usdm,
  HERO_TOKEN_ADDRESSES.usdc,
  HERO_TOKEN_ADDRESSES.usdt,
  HERO_TOKEN_ADDRESSES.dai,
  HERO_TOKEN_ADDRESSES.eurm,
  HERO_TOKEN_ADDRESSES.btc,
  HERO_TOKEN_ADDRESSES.eth,
]);

const TOKEN_DISPLAY_NAMES = new Map([
  [normalizeTokenKey(HERO_TOKEN_ADDRESSES.usdm), "USDm"],
  [normalizeTokenKey(HERO_TOKEN_ADDRESSES.usdc), "USDC"],
  [normalizeTokenKey(HERO_TOKEN_ADDRESSES.usdt), "USD₮"],
  [normalizeTokenKey(HERO_TOKEN_ADDRESSES.dai), "DAI"],
  [normalizeTokenKey(HERO_TOKEN_ADDRESSES.eurm), "EURm"],
  [normalizeTokenKey(HERO_TOKEN_ADDRESSES.btc), "BTC"],
  [normalizeTokenKey(HERO_TOKEN_ADDRESSES.eth), "ETH"],
]);

const QUOTE_DEBOUNCE_MS = 500;
const QUOTE_REFRESH_MS = 1000;
const PRICES_REFRESH_MS = 10000;
const MAX_ALLOWED_PRICE_IMPACT = 5;
const HERO_TAG_VISIBLE_MS = 3500;
const DEFAULT_SLIPPAGE_BPS = "100";

const elements = {
  tokenIn: document.getElementById("tokenIn"),
  tokenOut: document.getElementById("tokenOut"),
  tokenInTrigger: document.getElementById("tokenInTrigger"),
  tokenOutTrigger: document.getElementById("tokenOutTrigger"),
  tokenInIcon: document.getElementById("tokenInIcon"),
  tokenOutIcon: document.getElementById("tokenOutIcon"),
  tokenInLabel: document.getElementById("tokenInLabel"),
  tokenOutLabel: document.getElementById("tokenOutLabel"),

  amountIn: document.getElementById("amountIn"),
  amountOut: document.getElementById("amountOut"),
  amountInValue: document.getElementById("amountInValue"),
  amountOutValue: document.getElementById("amountOutValue"),
  priceImpactRow: document.getElementById("priceImpactRow"),

  swapButton: document.getElementById("swapButton"),
  connectWalletButton: document.getElementById("connectWalletButton"),

  heroTagView: document.getElementById("heroTagView"),
  heroBalanceView: document.getElementById("heroBalanceView"),
  heroBalanceTotal: document.getElementById("heroBalanceTotal"),
  heroBalanceUSDMAmount: document.getElementById("heroBalanceUSDMAmount"),
  heroBalanceUSDMUsd: document.getElementById("heroBalanceUSDMUsd"),
  heroBalanceUSDCAmount: document.getElementById("heroBalanceUSDCAmount"),
  heroBalanceUSDCUsd: document.getElementById("heroBalanceUSDCUsd"),
  heroBalanceUSDTAmount: document.getElementById("heroBalanceUSDTAmount"),
  heroBalanceUSDTUsd: document.getElementById("heroBalanceUSDTUsd"),
  heroBalanceDAIAmount: document.getElementById("heroBalanceDAIAmount"),
  heroBalanceDAIUsd: document.getElementById("heroBalanceDAIUsd"),
  heroBalanceEURMAmount: document.getElementById("heroBalanceEURMAmount"),
  heroBalanceEURMUsd: document.getElementById("heroBalanceEURMUsd"),
  heroBalanceBtc: document.getElementById("heroBalanceBtc"),
  heroBalanceBtcUsd: document.getElementById("heroBalanceBtcUsd"),
  heroBalanceEth: document.getElementById("heroBalanceEth"),
  heroBalanceEthUsd: document.getElementById("heroBalanceEthUsd"),
};

const modalElements = {
  modal: document.getElementById("tokenModal"),
  list: document.getElementById("tokenModalList"),
  close: document.getElementById("tokenModalClose"),
  title: document.getElementById("tokenModalTitle"),
  backdrop: document.querySelector(".token-modal-backdrop"),
};

let tokensState = [];
let inputTokensState = [];
let outputTokensState = [];
let tokenMap = new Map();

let walletState = {
  connected: false,
  address: "",
  chainId: "",
};

let balancesCache = new Map();

let currentBalanceInRaw = 0n;
let currentBalanceInDecimals = 18;

let quoteIntervalId = null;
let quoteDebounceTimer = null;
let isQuoteFetching = false;
let pendingQuoteRerun = false;
let lastSettledQuote = "";
let lastQuoteSignature = "";
let quoteRequestId = 0;
let latestAppliedQuoteRequestId = 0;

let pricesIntervalId = null;
let heroCycleTimeoutId = null;

let pricesState = {
  quoteToken: "",
  prices: new Map(),
};

let currentPriceImpact = null;
let currentQuoteHealth = "unknown";
let isSwapExecuting = false;
let buttonMessageOverride = "";

let activeTokenSelectTarget = null;

function normalizeTokenKey(value) {
  return typeof value === "string" ? value.toLowerCase() : "";
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function setButtonOverride(message) {
  buttonMessageOverride = message || "";
  if (elements.swapButton && buttonMessageOverride) {
    elements.swapButton.textContent = buttonMessageOverride;
  }
}

function clearButtonOverride() {
  buttonMessageOverride = "";
}

function setSwapButtonDangerState(isDanger) {
  if (!elements.swapButton) return;
  elements.swapButton.classList.toggle("swap-button-danger", !!isDanger);
}

function isHexString(value) {
  return typeof value === "string" && /^0x[0-9a-fA-F]*$/.test(value);
}

function hexToBigInt(value, fallback = 0n) {
  if (typeof value !== "string") return fallback;
  if (value === "0x" || value === "") return fallback;
  if (!isHexString(value)) return fallback;

  try {
    return BigInt(value);
  } catch {
    return fallback;
  }
}

function isValidAddress(value) {
  return typeof value === "string" && /^0x[a-fA-F0-9]{40}$/.test(value);
}

function normalizeTxValue(value) {
  if (typeof value === "string") {
    if (value === "") return "0x0";
    if (value.startsWith("0x")) return value;
    try {
      return `0x${BigInt(value).toString(16)}`;
    } catch {
      return "0x0";
    }
  }

  if (typeof value === "number") {
    return `0x${BigInt(value).toString(16)}`;
  }

  if (typeof value === "bigint") {
    return `0x${value.toString(16)}`;
  }

  return "0x0";
}

function getTokenAddress(token) {
  return token?.address || token?.Address || "";
}

function getTokenSymbol(token) {
  return token?.symbol || token?.Symbol || "Unknown";
}

function getTokenDecimals(token) {
  const value = token?.decimals ?? token?.Decimals;
  if (typeof value === "number") return value;
  if (typeof value === "string" && value !== "") return Number(value);
  return 18;
}

function getTokenValue(token) {
  const address = getTokenAddress(token);
  if (address) return address;
  return getTokenSymbol(token);
}

function getTokenLabel(token) {
  const address = normalizeTokenKey(getTokenAddress(token));
  return TOKEN_DISPLAY_NAMES.get(address) || getTokenSymbol(token);
}

function getTokenLogoUrl(tokenOrValue) {
  const symbol =
    typeof tokenOrValue === "string"
      ? tokenOrValue
      : getTokenLabel(tokenOrValue);

  if (!symbol) return DEFAULT_TOKEN_IMAGE;
  return `/public/assets/${String(symbol).toUpperCase()}.png`;
}

function isNativeToken(token) {
  if (!token) return false;

  if (token.isNative === true || token.IsNative === true) {
    return true;
  }

  const address = normalizeTokenKey(getTokenAddress(token));
  if (address === "0x0000000000000000000000000000000000000000") {
    return true;
  }

  const symbol = normalizeTokenKey(getTokenSymbol(token));
  return NATIVE_TOKEN_PLACEHOLDERS.has(symbol);
}

function isAllowedToken(token) {
  if (!token) return false;
  return ALLOWED_TOKEN_ADDRESSES.has(normalizeTokenKey(getTokenAddress(token)));
}

function populateTokenSelect(selectEl, tokens, selectedValue = "") {
  selectEl.innerHTML = "";

  for (const token of tokens) {
    const option = document.createElement("option");
    option.value = getTokenValue(token);
    option.textContent = getTokenLabel(token);

    if (selectedValue && option.value === selectedValue) {
      option.selected = true;
    }

    selectEl.appendChild(option);
  }
}

function rebuildTokenMap(tokens) {
  tokenMap = new Map();

  for (const token of tokens) {
    tokenMap.set(getTokenValue(token), token);
  }
}

function getSelectedToken(selectEl) {
  return tokenMap.get(selectEl.value) || null;
}

function findTokenByAddress(address) {
  const normalized = normalizeTokenKey(address);
  return (
    tokensState.find(
      (token) => normalizeTokenKey(getTokenAddress(token)) === normalized
    ) || null
  );
}

function isSameToken() {
  return (
    !!elements.tokenIn?.value &&
    !!elements.tokenOut?.value &&
    elements.tokenIn.value === elements.tokenOut.value
  );
}

function getDefaultInputToken(tokens) {
  const preferredAddresses = [
    HERO_TOKEN_ADDRESSES.usdm,
    HERO_TOKEN_ADDRESSES.usdc,
    HERO_TOKEN_ADDRESSES.usdt,
    HERO_TOKEN_ADDRESSES.dai,
    HERO_TOKEN_ADDRESSES.eurm,
    HERO_TOKEN_ADDRESSES.btc,
    HERO_TOKEN_ADDRESSES.eth,
  ];

  for (const address of preferredAddresses) {
    const match = tokens.find(
      (token) => normalizeTokenKey(getTokenAddress(token)) === address
    );
    if (match) return match;
  }

  return null;
}

function getDefaultOutputToken(tokens) {
  const preferredAddresses = [
    HERO_TOKEN_ADDRESSES.btc,
    HERO_TOKEN_ADDRESSES.eth,
    HERO_TOKEN_ADDRESSES.usdm,
    HERO_TOKEN_ADDRESSES.usdc,
    HERO_TOKEN_ADDRESSES.usdt,
    HERO_TOKEN_ADDRESSES.dai,
    HERO_TOKEN_ADDRESSES.eurm,
  ];

  for (const address of preferredAddresses) {
    const match = tokens.find(
      (token) => normalizeTokenKey(getTokenAddress(token)) === address
    );
    if (match) return match;
  }

  return null;
}

function loadTokenSelectors() {
  const defaultInToken = getDefaultInputToken(inputTokensState);
  const defaultOutToken = getDefaultOutputToken(outputTokensState);

  const selectedIn = defaultInToken ? getTokenValue(defaultInToken) : "";
  const selectedOut = defaultOutToken ? getTokenValue(defaultOutToken) : "";

  populateTokenSelect(elements.tokenIn, inputTokensState, selectedIn);
  populateTokenSelect(elements.tokenOut, outputTokensState, selectedOut);

  syncTriggerWithSelection("in");
  syncTriggerWithSelection("out");
}

function formatWalletAddress(address) {
  if (!address || typeof address !== "string") return "Connected";
  if (address.length < 10) return address;
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
}

function getConnectedIdentityLabel() {
  return formatWalletAddress(walletState.address);
}

function updateConnectWalletButton() {
  if (!elements.connectWalletButton) return;

  if (!walletState.connected) {
    elements.connectWalletButton.textContent = "Connect";
    return;
  }

  elements.connectWalletButton.textContent = getConnectedIdentityLabel();
}

function updateSwapButtonState() {
  const hasInput = inputTokensState.length > 0;
  const hasOutput = outputTokensState.length > 0;
  const hasWallet = walletState.connected;

  elements.swapButton.disabled = !(hasInput && hasOutput && hasWallet);
}

function updateSwapButtonStateWithBalanceCheck() {
  if (isSwapExecuting) {
    elements.swapButton.disabled = true;
    setSwapButtonDangerState(false);
    return;
  }

  if (buttonMessageOverride) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = buttonMessageOverride;
    setSwapButtonDangerState(false);
    return;
  }

  if (!walletState.connected) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Connect";
    setSwapButtonDangerState(false);
    return;
  }

  const tokenIn = getSelectedToken(elements.tokenIn);
  const tokenOut = getSelectedToken(elements.tokenOut);
  const amount = String(elements.amountIn.value || "").trim();

  if (!tokenIn || !tokenOut) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Select Tokens";
    setSwapButtonDangerState(false);
    return;
  }

  if (isSameToken()) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Select Different Tokens";
    setSwapButtonDangerState(false);
    return;
  }

  if (!amount) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Enter Amount";
    setSwapButtonDangerState(false);
    return;
  }

  let amountRaw;
  try {
    amountRaw = parseUnits(amount, currentBalanceInDecimals);
  } catch {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Invalid Amount";
    setSwapButtonDangerState(false);
    return;
  }

  if (amountRaw <= 0n) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Enter Amount";
    setSwapButtonDangerState(false);
    return;
  }

  if (amountRaw > currentBalanceInRaw) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = `Insufficient ${getTokenLabel(tokenIn)}`;
    setSwapButtonDangerState(false);
    return;
  }

  if (!String(elements.amountOut.value || "").trim()) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Loading Quote";
    setSwapButtonDangerState(false);
    return;
  }

  if (currentQuoteHealth === "low-liquidity") {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = `Low liquidity for ${getTokenLabel(tokenOut)}`;
    setSwapButtonDangerState(true);
    return;
  }

  if (
    currentQuoteHealth === "high-impact" ||
    (typeof currentPriceImpact === "number" &&
      currentPriceImpact > MAX_ALLOWED_PRICE_IMPACT)
  ) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = `High impact on ${getTokenLabel(tokenOut)}`;
    setSwapButtonDangerState(true);
    return;
  }

  elements.swapButton.disabled = false;
  elements.swapButton.textContent = "Convert";
  setSwapButtonDangerState(false);
}

function setHeroTokenRow(amountEl, usdEl, amountText = "0", usdText = "$0.00") {
  if (amountEl) amountEl.textContent = amountText;
  if (usdEl) usdEl.textContent = usdText;
}

function resetDisplayedBalances() {
  currentBalanceInRaw = 0n;
  currentBalanceInDecimals = 18;

  if (elements.heroBalanceTotal) {
    elements.heroBalanceTotal.textContent = "$0.00";
  }

  setHeroTokenRow(elements.heroBalanceUSDMAmount, elements.heroBalanceUSDMUsd);
  setHeroTokenRow(elements.heroBalanceUSDCAmount, elements.heroBalanceUSDCUsd);
  setHeroTokenRow(elements.heroBalanceUSDTAmount, elements.heroBalanceUSDTUsd);
  setHeroTokenRow(elements.heroBalanceDAIAmount, elements.heroBalanceDAIUsd);
  setHeroTokenRow(elements.heroBalanceEURMAmount, elements.heroBalanceEURMUsd);
  setHeroTokenRow(elements.heroBalanceBtc, elements.heroBalanceBtcUsd);
  setHeroTokenRow(elements.heroBalanceEth, elements.heroBalanceEthUsd);
}

function clearPriceImpactDisplay() {
  currentPriceImpact = null;
  currentQuoteHealth = "unknown";
  if (!elements.priceImpactRow) return;
  elements.priceImpactRow.textContent = "";
  elements.priceImpactRow.className = "price-impact";
}

function resetQuoteDisplay() {
  lastSettledQuote = "";
  lastQuoteSignature = "";
  latestAppliedQuoteRequestId = 0;
  elements.amountOut.value = "";
  clearPriceImpactDisplay();
  updateDisplayedTokenValues();
}

function formatUnits(value, decimals = 18, maxFractionDigits = 12) {
  const big = typeof value === "bigint" ? value : BigInt(value);
  const safeDecimals = Number.isFinite(decimals) ? decimals : 18;
  const base = 10n ** BigInt(safeDecimals);
  const whole = big / base;
  const fraction = big % base;

  let wholeStr = whole.toString();

  if (fraction === 0n || maxFractionDigits === 0) {
    return wholeStr;
  }

  let fractionStr = fraction.toString().padStart(safeDecimals, "0");
  fractionStr = fractionStr.replace(/0+$/, "");

  if (!fractionStr) {
    return wholeStr;
  }

  if (
    Number.isFinite(maxFractionDigits) &&
    maxFractionDigits > 0 &&
    fractionStr.length > maxFractionDigits
  ) {
    fractionStr = fractionStr.slice(0, maxFractionDigits).replace(/0+$/, "");
  }

  if (!fractionStr) {
    return wholeStr;
  }

  return `${wholeStr}.${fractionStr}`;
}

function addThousandsSeparatorsToDecimalString(value) {
  const str = String(value || "").trim();
  if (!str) return "";

  const negative = str.startsWith("-");
  const unsigned = negative ? str.slice(1) : str;

  const [whole, fraction] = unsigned.split(".");
  const wholeWithCommas = (whole || "0").replace(/\B(?=(\d{3})+(?!\d))/g, ",");

  if (fraction && fraction.length > 0) {
    return `${negative ? "-" : ""}${wholeWithCommas}.${fraction}`;
  }

  return `${negative ? "-" : ""}${wholeWithCommas}`;
}

function formatTokenDisplayStringFromRaw(rawValue, decimals = 18, maxFractionDigits = 12) {
  const normalized = formatUnits(rawValue, decimals, maxFractionDigits);
  return addThousandsSeparatorsToDecimalString(normalized);
}

function parseUnits(value, decimals = 18) {
  const normalized = String(value).trim();

  if (!normalized) {
    return 0n;
  }

  if (!/^\d*\.?\d*$/.test(normalized)) {
    throw new Error(`invalid numeric input: ${value}`);
  }

  const [whole = "0", fraction = ""] = normalized.split(".");
  const safeDecimals = Number.isFinite(decimals) ? decimals : 18;
  const paddedFraction = (fraction + "0".repeat(safeDecimals)).slice(0, safeDecimals);

  return (
    BigInt(whole || "0") * 10n ** BigInt(safeDecimals) +
    BigInt(paddedFraction || "0")
  );
}

function formatDollarValue(value) {
  if (!Number.isFinite(value) || value <= 0) return "";

  if (value >= 1000000) {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      notation: "compact",
      maximumFractionDigits: 2,
    }).format(value);
  }

  if (value >= 1) {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  }

  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  }).format(value);
}

function parseFormattedAmount(formatted) {
  const value = Number(formatted);
  return Number.isFinite(value) ? value : 0;
}

function getTokenPrice(token) {
  if (!token) return null;
  const key = normalizeTokenKey(getTokenAddress(token) || getTokenValue(token));
  if (!key) return null;
  return pricesState.prices.get(key) ?? null;
}

function getTokenUsdValueFromBalanceResult(result, token) {
  const price = getTokenPrice(token);
  if (!Number.isFinite(price)) return 0;

  const amountNumeric = parseFormattedAmount(result.formatted);
  if (!Number.isFinite(amountNumeric) || amountNumeric <= 0) return 0;

  return amountNumeric * price;
}

function computePriceImpact({ amountIn, amountOut, priceIn, priceOut }) {
  if (
    !Number.isFinite(amountIn) ||
    amountIn <= 0 ||
    !Number.isFinite(amountOut) ||
    amountOut <= 0 ||
    !Number.isFinite(priceIn) ||
    priceIn <= 0 ||
    !Number.isFinite(priceOut) ||
    priceOut <= 0
  ) {
    return null;
  }

  const expectedOut = (amountIn * priceIn) / priceOut;
  if (!Number.isFinite(expectedOut) || expectedOut <= 0) return null;

  return Math.max(0, ((expectedOut - amountOut) / expectedOut) * 100);
}

function renderPriceImpact(impact) {
  currentPriceImpact = Number.isFinite(impact) ? impact : null;

  if (currentPriceImpact === null) {
    currentQuoteHealth = "unknown";
  } else if (currentPriceImpact > MAX_ALLOWED_PRICE_IMPACT) {
    currentQuoteHealth = "high-impact";
  } else {
    currentQuoteHealth = "ok";
  }

  if (!elements.priceImpactRow) return;
  elements.priceImpactRow.textContent = "";
  elements.priceImpactRow.className = "price-impact";
}

function updateDisplayedTokenValues() {
  const tokenIn = getSelectedToken(elements.tokenIn);
  const tokenOut = getSelectedToken(elements.tokenOut);

  const amountIn = Number(String(elements.amountIn.value || "").replace(/,/g, ""));
  const amountOut = Number(String(elements.amountOut.value || "").replace(/,/g, ""));

  const priceIn = getTokenPrice(tokenIn);
  const priceOut = getTokenPrice(tokenOut);

  if (elements.amountInValue) {
    if (
      Number.isFinite(amountIn) &&
      amountIn > 0 &&
      Number.isFinite(priceIn)
    ) {
      elements.amountInValue.textContent = formatDollarValue(amountIn * priceIn);
    } else {
      elements.amountInValue.textContent = "";
    }
  }

  if (elements.amountOutValue) {
    if (
      Number.isFinite(amountOut) &&
      amountOut > 0 &&
      Number.isFinite(priceOut)
    ) {
      elements.amountOutValue.textContent = formatDollarValue(amountOut * priceOut);
    } else {
      elements.amountOutValue.textContent = "";
    }
  }

  const currentSignature = getCurrentQuoteSignature();
  const hasValidSettledQuote =
    !!lastSettledQuote &&
    !!lastQuoteSignature &&
    lastQuoteSignature === currentSignature &&
    latestAppliedQuoteRequestId > 0 &&
    String(elements.amountOut.value || "").trim() !== "" &&
    String(elements.amountOut.value || "").trim() !== "Loading...";

  if (!hasValidSettledQuote) {
    clearPriceImpactDisplay();
    return;
  }

  const impact = computePriceImpact({
    amountIn,
    amountOut,
    priceIn,
    priceOut,
  });

  renderPriceImpact(impact);
}

function debounceQuoteFetch() {
  window.clearTimeout(quoteDebounceTimer);
  quoteDebounceTimer = window.setTimeout(() => {
    fetchQuote();
  }, QUOTE_DEBOUNCE_MS);
}

async function rpcCall(method, params) {
  return await window.ethereum.request({ method, params });
}

function encodeBalanceOf(address) {
  const selector = "0x70a08231";
  const stripped = address.toLowerCase().replace(/^0x/, "");
  return selector + stripped.padStart(64, "0");
}

function encodeDecimals() {
  return "0x313ce567";
}

async function ethCall(to, data) {
  return await rpcCall("eth_call", [{ to, data }, "latest"]);
}

async function readERC20Decimals(token) {
  const tokenAddress = getTokenAddress(token);

  if (!isValidAddress(tokenAddress)) {
    return getTokenDecimals(token);
  }

  try {
    const raw = await ethCall(tokenAddress, encodeDecimals());
    const parsed = hexToBigInt(raw, BigInt(getTokenDecimals(token)));
    return Number(parsed);
  } catch {
    return getTokenDecimals(token);
  }
}

async function readERC20Balance(token, walletAddress) {
  const tokenAddress = getTokenAddress(token);

  if (!isValidAddress(tokenAddress)) {
    throw new Error(`invalid token address: ${tokenAddress}`);
  }

  if (!isValidAddress(walletAddress)) {
    throw new Error(`invalid wallet address: ${walletAddress}`);
  }

  const raw = await ethCall(tokenAddress, encodeBalanceOf(walletAddress));
  return hexToBigInt(raw, 0n);
}

async function readNativeBalance(walletAddress) {
  if (!isValidAddress(walletAddress)) {
    throw new Error(`invalid wallet address: ${walletAddress}`);
  }

  const raw = await rpcCall("eth_getBalance", [walletAddress, "latest"]);
  return hexToBigInt(raw, 0n);
}

async function readTokenBalance(token, walletAddress) {
  if (isNativeToken(token)) {
    const rawBalance = await readNativeBalance(walletAddress);
    return {
      rawBalance,
      decimals: 18,
      formatted: formatUnits(rawBalance, 18),
    };
  }

  const [rawBalance, decimals] = await Promise.all([
    readERC20Balance(token, walletAddress),
    readERC20Decimals(token),
  ]);

  return {
    rawBalance,
    decimals,
    formatted: formatUnits(rawBalance, decimals),
  };
}

function getBalanceCacheKey(token, walletAddress) {
  return `${normalizeTokenKey(walletAddress)}:${normalizeTokenKey(
    walletState.chainId
  )}:${normalizeTokenKey(getTokenAddress(token) || getTokenValue(token))}`;
}

function clearBalanceCache() {
  balancesCache = new Map();
}

async function getBalanceForToken(token, walletAddress) {
  const key = getBalanceCacheKey(token, walletAddress);

  if (balancesCache.has(key)) {
    return balancesCache.get(key);
  }

  const result = await readTokenBalance(token, walletAddress);
  balancesCache.set(key, result);
  return result;
}

async function refreshInputBalanceState() {
  if (!walletState.connected || !walletState.address) {
    currentBalanceInRaw = 0n;
    currentBalanceInDecimals = 18;
    return;
  }

  const token = getSelectedToken(elements.tokenIn);
  if (!token) {
    currentBalanceInRaw = 0n;
    currentBalanceInDecimals = 18;
    return;
  }

  try {
    const { rawBalance, decimals } = await getBalanceForToken(
      token,
      walletState.address
    );
    currentBalanceInRaw = rawBalance;
    currentBalanceInDecimals = decimals;
  } catch (error) {
    console.error("failed to refresh input balance state:", error);
    currentBalanceInRaw = 0n;
    currentBalanceInDecimals = getTokenDecimals(token);
  }
}

async function updateHeroBalances() {
  if (!elements.heroBalanceTotal) return;

  if (!walletState.connected || !walletState.address) {
    resetDisplayedBalances();
    return;
  }

  let totalUsd = 0;

  const heroRows = [
    {
      key: "usdm",
      amountEl: elements.heroBalanceUSDMAmount,
      usdEl: elements.heroBalanceUSDMUsd,
      fractionDigits: 2,
    },
    {
      key: "usdc",
      amountEl: elements.heroBalanceUSDCAmount,
      usdEl: elements.heroBalanceUSDCUsd,
      fractionDigits: 2,
    },
    {
      key: "usdt",
      amountEl: elements.heroBalanceUSDTAmount,
      usdEl: elements.heroBalanceUSDTUsd,
      fractionDigits: 2,
    },
    {
      key: "dai",
      amountEl: elements.heroBalanceDAIAmount,
      usdEl: elements.heroBalanceDAIUsd,
      fractionDigits: 2,
    },
    {
      key: "eurm",
      amountEl: elements.heroBalanceEURMAmount,
      usdEl: elements.heroBalanceEURMUsd,
      fractionDigits: 2,
    },
    {
      key: "btc",
      amountEl: elements.heroBalanceBtc,
      usdEl: elements.heroBalanceBtcUsd,
      fractionDigits: 6,
    },
    {
      key: "eth",
      amountEl: elements.heroBalanceEth,
      usdEl: elements.heroBalanceEthUsd,
      fractionDigits: 6,
    },
  ];

  for (const row of heroRows) {
    const token = findTokenByAddress(HERO_TOKEN_ADDRESSES[row.key]);

    if (!token) {
      setHeroTokenRow(row.amountEl, row.usdEl);
      continue;
    }

    try {
      const result = await getBalanceForToken(token, walletState.address);

      const displayAmount = addThousandsSeparatorsToDecimalString(
        formatUnits(result.rawBalance, result.decimals, row.fractionDigits)
      );

      const usdValue = getTokenUsdValueFromBalanceResult(result, token);
      totalUsd += usdValue;

      setHeroTokenRow(
        row.amountEl,
        row.usdEl,
        displayAmount,
        formatDollarValue(usdValue) || "$0.00"
      );
    } catch (error) {
      console.error(`failed to update hero balance for ${row.key}:`, error);
      setHeroTokenRow(row.amountEl, row.usdEl);
    }
  }

  elements.heroBalanceTotal.textContent = formatDollarValue(totalUsd) || "$0.00";
}

async function refreshDisplayedBalances() {
  await refreshInputBalanceState();
  await updateHeroBalances();
  updateSwapButtonStateWithBalanceCheck();
}

async function refreshDisplayedBalancesWithRetry(attempts = 4, delayMs = 700) {
  let lastError = null;

  for (let i = 0; i < attempts; i += 1) {
    try {
      clearBalanceCache();
      await refreshDisplayedBalances();
      return;
    } catch (error) {
      lastError = error;
      console.error(`balance refresh attempt ${i + 1} failed:`, error);

      if (i < attempts - 1) {
        await sleep(delayMs);
      }
    }
  }

  if (lastError) {
    throw lastError;
  }
}

function getCurrentQuoteContext() {
  const tokenIn = getSelectedToken(elements.tokenIn);
  const tokenOut = getSelectedToken(elements.tokenOut);
  const amountIn = String(elements.amountIn.value || "").trim();

  if (!tokenIn || !tokenOut || !amountIn || isSameToken()) {
    return null;
  }

  return {
    tokenIn,
    tokenOut,
    tokenInValue: getTokenValue(tokenIn),
    tokenOutValue: getTokenValue(tokenOut),
    amountIn,
  };
}

function getCurrentQuoteSignature() {
  const context = getCurrentQuoteContext();
  if (!context) return "";

  return `${context.tokenInValue}:${context.tokenOutValue}:${context.amountIn}`;
}

async function fetchQuote() {
  if (isSwapExecuting) {
    return;
  }

  const context = getCurrentQuoteContext();

  if (!context) {
    resetQuoteDisplay();
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  const signature = `${context.tokenInValue}:${context.tokenOutValue}:${context.amountIn}`;

  if (isQuoteFetching) {
    pendingQuoteRerun = true;
    return;
  }

  isQuoteFetching = true;
  pendingQuoteRerun = false;

  const requestId = ++quoteRequestId;
  const decimalsIn = getTokenDecimals(context.tokenIn);
  const decimalsOut = getTokenDecimals(context.tokenOut);

  let amountInRaw;
  try {
    amountInRaw = parseUnits(context.amountIn, decimalsIn);
  } catch {
    resetQuoteDisplay();
    updateSwapButtonStateWithBalanceCheck();
    isQuoteFetching = false;
    return;
  }

  if (amountInRaw <= 0n) {
    resetQuoteDisplay();
    updateSwapButtonStateWithBalanceCheck();
    isQuoteFetching = false;
    return;
  }

  try {
    const params = new URLSearchParams({
      tokenIn: context.tokenInValue,
      tokenOut: context.tokenOutValue,
      amountIn: amountInRaw.toString(),
    });

    const url = `/quote?${params.toString()}`;

    if (!lastSettledQuote) {
      elements.amountOut.value = "Loading...";
      updateDisplayedTokenValues();
      updateSwapButtonStateWithBalanceCheck();
    }

    const response = await fetch(url, {
      method: "GET",
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      const body = await response.text();
      console.error("quote http error:", response.status, body);
      throw new Error(`quote failed with HTTP ${response.status}`);
    }

    const data = await response.json();
    const raw = data?.AmountOut ?? data?.amountOut ?? data?.amount_out;

    if (typeof raw !== "string") {
      console.error("quote response missing AmountOut string:", data);
      throw new Error("quote response missing AmountOut");
    }

    const amountOutRaw = BigInt(raw);
    if (amountOutRaw <= 0n) {
      currentQuoteHealth = "low-liquidity";
    }

    const currentSignature = getCurrentQuoteSignature();
    if (requestId !== quoteRequestId || currentSignature !== signature) {
      return;
    }

    const formatted = formatUnits(amountOutRaw, decimalsOut, 12);
    const displayFormatted = formatTokenDisplayStringFromRaw(
      amountOutRaw,
      decimalsOut,
      12
    );

    latestAppliedQuoteRequestId = requestId;
    lastSettledQuote = formatted;
    lastQuoteSignature = signature;
    elements.amountOut.value = displayFormatted;

    updateDisplayedTokenValues();

    if (amountOutRaw <= 0n) {
      currentQuoteHealth = "low-liquidity";
    }
  } catch (error) {
    console.error("quote failed:", error);

    const currentSignature = getCurrentQuoteSignature();
    const isStillCurrent =
      requestId === quoteRequestId && currentSignature === signature;

    if (isStillCurrent && lastSettledQuote && lastQuoteSignature === signature) {
      latestAppliedQuoteRequestId = requestId;
      elements.amountOut.value = addThousandsSeparatorsToDecimalString(
        lastSettledQuote
      );
    } else if (isStillCurrent) {
      elements.amountOut.value = "";
      currentQuoteHealth = "low-liquidity";
      clearPriceImpactDisplay();
    }

    updateDisplayedTokenValues();
  } finally {
    isQuoteFetching = false;
    updateSwapButtonStateWithBalanceCheck();

    if (pendingQuoteRerun && !isSwapExecuting) {
      pendingQuoteRerun = false;
      fetchQuote();
    }
  }
}

function startQuoteRefreshLoop() {
  stopQuoteRefreshLoop();
  quoteIntervalId = window.setInterval(() => {
    if (isSwapExecuting) return;
    const context = getCurrentQuoteContext();
    if (!context) return;
    fetchQuote();
  }, QUOTE_REFRESH_MS);
}

function stopQuoteRefreshLoop() {
  if (quoteIntervalId !== null) {
    window.clearInterval(quoteIntervalId);
    quoteIntervalId = null;
  }
}

async function fetchPrices() {
  try {
    const response = await fetch("/prices", {
      method: "GET",
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const data = await response.json();
    const rawPrices = data?.prices ?? data?.Prices ?? {};

    const nextPrices = new Map();
    for (const [address, price] of Object.entries(rawPrices)) {
      const numericPrice = Number(price);
      if (Number.isFinite(numericPrice)) {
        nextPrices.set(normalizeTokenKey(address), numericPrice);
      }
    }

    pricesState = {
      quoteToken: data?.quote_token ?? data?.QuoteToken ?? "",
      prices: nextPrices,
    };

    updateDisplayedTokenValues();

    if (walletState.connected) {
      await updateHeroBalances();
    }

    updateSwapButtonStateWithBalanceCheck();
  } catch (error) {
    console.error("failed to load prices:", error);
  }
}

function startPricesRefreshLoop() {
  stopPricesRefreshLoop();
  pricesIntervalId = window.setInterval(() => {
    fetchPrices();
  }, PRICES_REFRESH_MS);
}

function stopPricesRefreshLoop() {
  if (pricesIntervalId !== null) {
    window.clearInterval(pricesIntervalId);
    pricesIntervalId = null;
  }
}

function filterInputTokens(tokens) {
  return tokens.filter(isAllowedToken);
}

function filterOutputTokens(tokens) {
  return tokens.filter(isAllowedToken);
}

function loadTokenModalTitle(target) {
  if (!modalElements.title) return;
  modalElements.title.textContent =
    target === "in" ? "Select asset to pay" : "Select asset to receive";
}

function openTokenModal(target) {
  activeTokenSelectTarget = target;
  loadTokenModalTitle(target);
  renderTokenModalList(target);

  modalElements.modal.classList.remove("hidden");
  modalElements.modal.setAttribute("aria-hidden", "false");

  const trigger = target === "in" ? elements.tokenInTrigger : elements.tokenOutTrigger;
  trigger.setAttribute("aria-expanded", "true");

  document.body.style.overflow = "hidden";
}

function closeTokenModal() {
  modalElements.modal.classList.add("hidden");
  modalElements.modal.setAttribute("aria-hidden", "true");

  elements.tokenInTrigger.setAttribute("aria-expanded", "false");
  elements.tokenOutTrigger.setAttribute("aria-expanded", "false");

  activeTokenSelectTarget = null;
  document.body.style.overflow = "";
}

function getTokenPickerCaption(token) {
  const address = normalizeTokenKey(getTokenAddress(token));

  if (STABLE_TOKEN_ADDRESSES.has(address)) {
    return "Supported stable";
  }

  if (
    address === HERO_TOKEN_ADDRESSES.btc ||
    address === HERO_TOKEN_ADDRESSES.eth
  ) {
    return "Supported asset";
  }

  return "Supported token";
}

function renderTokenModalList(target) {
  const tokens = target === "in" ? inputTokensState : outputTokensState;
  const selectEl = target === "in" ? elements.tokenIn : elements.tokenOut;

  modalElements.list.innerHTML = "";

  for (const token of tokens) {
    if (!isAllowedToken(token)) continue;

    const isActive = selectEl.value === getTokenValue(token);

    const item = document.createElement("button");
    item.type = "button";
    item.className = "token-modal-item";

    if (isActive) {
      item.classList.add("active");
    }

    item.innerHTML = `
      <span class="token-modal-item-left">
        <span class="token-modal-item-icon">
          <img src="${getTokenLogoUrl(token)}" alt="" />
        </span>
        <span class="token-modal-item-meta">
          <span class="token-modal-item-symbol">${getTokenLabel(token)}</span>
          <span class="token-modal-item-caption">${getTokenPickerCaption(token)}</span>
        </span>
      </span>
      <span class="token-modal-item-check">✓</span>
    `;

    item.addEventListener("click", async () => {
      selectEl.value = getTokenValue(token);
      syncTriggerWithSelection(target);
      closeTokenModal();

      resetQuoteDisplay();

      if (walletState.connected) {
        await refreshDisplayedBalances();
      }

      updateDisplayedTokenValues();
      updateSwapButtonStateWithBalanceCheck();
      await fetchQuote();
    });

    modalElements.list.appendChild(item);
  }
}

async function loadTokens() {
  setButtonOverride("Loading tokens...");

  try {
    const response = await fetch("/tokens", {
      method: "GET",
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const tokens = await response.json();

    if (!Array.isArray(tokens) || tokens.length === 0) {
      throw new Error("No tokens returned");
    }

    tokensState = tokens;
    inputTokensState = filterInputTokens(tokensState);
    outputTokensState = filterOutputTokens(tokensState);

    if (inputTokensState.length === 0 || outputTokensState.length === 0) {
      throw new Error("Required allowed tokens are unavailable");
    }

    rebuildTokenMap(tokensState);
    loadTokenSelectors();

    elements.amountIn.value = DEFAULT_AMOUNT_IN;

    clearButtonOverride();
    updateSwapButtonState();

    if (walletState.connected) {
      await refreshDisplayedBalances();
    }

    updateDisplayedTokenValues();
    await fetchQuote();
    updateSwapButtonStateWithBalanceCheck();
  } catch (error) {
    console.error("failed to load tokens:", error);
    setButtonOverride("Unavailable");
    elements.swapButton.disabled = true;
    setSwapButtonDangerState(false);
  }
}

async function syncWalletStateFromProvider() {
  const [accounts, chainId] = await Promise.all([
    rpcCall("eth_accounts", []),
    rpcCall("eth_chainId", []),
  ]);

  walletState.connected = Array.isArray(accounts) && accounts.length > 0;
  walletState.address = walletState.connected ? accounts[0] : "";
  walletState.chainId = chainId || "";

  updateConnectWalletButton();
  updateSwapButtonState();

  if (walletState.connected) {
    clearBalanceCache();
    clearButtonOverride();
    await refreshDisplayedBalances();
  } else {
    resetDisplayedBalances();
  }

  updateSwapButtonStateWithBalanceCheck();
}

async function connectWallet() {
  if (!window.ethereum) {
    setButtonOverride("No Wallet");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  try {
    setButtonOverride("Connecting...");
    updateSwapButtonStateWithBalanceCheck();

    const accounts = await rpcCall("eth_requestAccounts", []);

    if (!accounts || accounts.length === 0) {
      throw new Error("No accounts returned");
    }

    clearButtonOverride();
    await syncWalletStateFromProvider();
  } catch (error) {
    console.error("wallet connection failed:", error);
    setButtonOverride("Connection Failed");
    updateSwapButtonStateWithBalanceCheck();

    setTimeout(() => {
      clearButtonOverride();
      updateSwapButtonStateWithBalanceCheck();
    }, 2000);
  }
}

async function waitForTransactionReceipt(txHash, pollMs = 1200) {
  while (true) {
    const receipt = await window.ethereum.request({
      method: "eth_getTransactionReceipt",
      params: [txHash],
    });

    if (receipt) {
      return receipt;
    }

    await sleep(pollMs);
  }
}

function normalizeTxShape(tx) {
  if (!tx || typeof tx !== "object") return null;

  const to = tx.to || tx.To;
  const data = tx.data || tx.Data || "0x";
  const value = tx.value ?? tx.Value ?? "0x0";

  if (!to) return null;

  return {
    to,
    data,
    value,
  };
}

function extractSwapTransactions(data) {
  if (Array.isArray(data) && data.length > 0) {
    return data.map(normalizeTxShape).filter(Boolean);
  }

  const candidates = [
    data?.transactions,
    data?.Transactions,
    data?.txs,
    data?.Txs,
    data?.swap?.transactions,
    data?.swap?.Transactions,
  ];

  for (const candidate of candidates) {
    if (Array.isArray(candidate) && candidate.length > 0) {
      return candidate.map(normalizeTxShape).filter(Boolean);
    }
  }

  const singleCandidates = [
    data?.transaction,
    data?.Transaction,
    data?.tx,
    data?.Tx,
    data?.swap?.transaction,
    data?.swap?.Transaction,
  ];

  for (const candidate of singleCandidates) {
    const normalized = normalizeTxShape(candidate);
    if (normalized) {
      return [normalized];
    }
  }

  return [];
}

async function executeSwapPlan() {
  if (isSwapExecuting) return;

  const tokenIn = getSelectedToken(elements.tokenIn);
  const tokenOut = getSelectedToken(elements.tokenOut);
  const amountInHuman = String(elements.amountIn.value || "").trim();

  if (!walletState.connected || !walletState.address) {
    setButtonOverride("Connect");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (!tokenIn || !tokenOut) {
    setButtonOverride("Select Tokens");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (isSameToken()) {
    setButtonOverride("Select Different Tokens");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (!amountInHuman) {
    setButtonOverride("Enter Amount");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (currentQuoteHealth === "low-liquidity") {
    setButtonOverride(`Low liquidity for ${getTokenLabel(tokenOut)}`);
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (
    currentQuoteHealth === "high-impact" ||
    (typeof currentPriceImpact === "number" &&
      currentPriceImpact > MAX_ALLOWED_PRICE_IMPACT)
  ) {
    setButtonOverride(`High impact on ${getTokenLabel(tokenOut)}`);
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  isSwapExecuting = true;
  stopQuoteRefreshLoop();

  try {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Preparing...";

    const amountInRaw = parseUnits(amountInHuman, getTokenDecimals(tokenIn));

    const params = new URLSearchParams({
      user: walletState.address,
      receiver: walletState.address,
      tokenIn: getTokenValue(tokenIn),
      tokenOut: getTokenValue(tokenOut),
      amountIn: amountInRaw.toString(),
      slippageBps: DEFAULT_SLIPPAGE_BPS,
    });

    const response = await fetch(`/swap?${params.toString()}`, {
      method: "GET",
      cache: "no-store",
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      const body = await response.text();
      console.error("swap http error:", response.status, body);
      throw new Error(body || `swap failed with HTTP ${response.status}`);
    }

    const data = await response.json();
    const txs = extractSwapTransactions(data);

    if (!Array.isArray(txs) || txs.length === 0) {
      console.error("unexpected /swap response payload:", data);
      const backendMessage =
        data?.error ||
        data?.message ||
        data?.Error ||
        data?.Message ||
        "No transactions returned from /swap";
      throw new Error(backendMessage);
    }

    for (let i = 0; i < txs.length; i += 1) {
      const tx = txs[i];

      elements.swapButton.textContent =
        txs.length > 1 ? `Confirm ${i + 1}/${txs.length}` : "Confirm";

      const txHash = await window.ethereum.request({
        method: "eth_sendTransaction",
        params: [
          {
            from: walletState.address,
            to: tx.to,
            data: tx.data,
            value: normalizeTxValue(tx.value),
          },
        ],
      });

      elements.swapButton.textContent =
        txs.length > 1 ? `Pending ${i + 1}/${txs.length}` : "Pending";

      const receipt = await waitForTransactionReceipt(txHash);

      if (receipt.status === "0x0") {
        throw new Error(`Transaction reverted: ${txHash}`);
      }

      await refreshDisplayedBalancesWithRetry(3, 500);
      await fetchPrices();
      await fetchQuote();
    }

    elements.swapButton.textContent = "Complete";
    await sleep(1000);
  } catch (error) {
    console.error("swap execution failed:", error);

    const errorMessage = String(error?.message || "").toLowerCase();
    if (
      errorMessage.includes("user rejected") ||
      errorMessage.includes("user denied")
    ) {
      elements.swapButton.textContent = "Rejected";
    } else {
      elements.swapButton.textContent = "Failed";
    }

    await sleep(1200);
  } finally {
    isSwapExecuting = false;
    clearButtonOverride();
    updateSwapButtonStateWithBalanceCheck();
    startQuoteRefreshLoop();
  }
}

function syncTriggerWithSelection(target) {
  const selectEl = target === "in" ? elements.tokenIn : elements.tokenOut;
  const labelEl = target === "in" ? elements.tokenInLabel : elements.tokenOutLabel;
  const iconEl = target === "in" ? elements.tokenInIcon : elements.tokenOutIcon;

  const token = getSelectedToken(selectEl);

  if (!token) {
    labelEl.textContent = "Select";
    iconEl.innerHTML = `<img src="${DEFAULT_TOKEN_IMAGE}" alt="" />`;
    return;
  }

  labelEl.textContent = getTokenLabel(token);
  iconEl.innerHTML = `<img src="${getTokenLogoUrl(token)}" alt="" />`;
}

function showHeroView(view) {
  const showTag = view === "tag";

  if (elements.heroTagView) {
    elements.heroTagView.classList.toggle("hero-view-active", showTag);
    elements.heroTagView.setAttribute("aria-hidden", showTag ? "false" : "true");
  }

  if (elements.heroBalanceView) {
    elements.heroBalanceView.classList.toggle("hero-view-active", !showTag);
    elements.heroBalanceView.setAttribute("aria-hidden", showTag ? "true" : "false");
  }
}

function startHeroCycle() {
  if (heroCycleTimeoutId !== null) {
    window.clearTimeout(heroCycleTimeoutId);
    heroCycleTimeoutId = null;
  }

  if (walletState.connected) {
    showHeroView("balance");
    return;
  }

  showHeroView("tag");

  heroCycleTimeoutId = window.setTimeout(() => {
    showHeroView("balance");
  }, HERO_TAG_VISIBLE_MS);
}

function bindEvents() {
  elements.connectWalletButton.addEventListener("click", async () => {
    if (walletState.connected) return;
    await connectWallet();
  });

  elements.tokenInTrigger.addEventListener("click", (event) => {
    event.stopPropagation();
    openTokenModal("in");
  });

  elements.tokenOutTrigger.addEventListener("click", (event) => {
    event.stopPropagation();
    openTokenModal("out");
  });

  elements.tokenIn.addEventListener("change", async () => {
    syncTriggerWithSelection("in");
    resetQuoteDisplay();

    if (walletState.connected) {
      await refreshDisplayedBalances();
    }

    updateDisplayedTokenValues();
    updateSwapButtonStateWithBalanceCheck();
    await fetchQuote();
  });

  elements.tokenOut.addEventListener("change", async () => {
    syncTriggerWithSelection("out");
    resetQuoteDisplay();

    if (walletState.connected) {
      await refreshDisplayedBalances();
    }

    updateDisplayedTokenValues();
    updateSwapButtonStateWithBalanceCheck();
    await fetchQuote();
  });

  elements.amountIn.addEventListener("input", () => {
    const sanitized = String(elements.amountIn.value || "")
      .replace(/[^\d.]/g, "")
      .replace(/^(\d*\.?\d*).*$/, "$1");

    if (elements.amountIn.value !== sanitized) {
      elements.amountIn.value = sanitized;
    }

    resetQuoteDisplay();
    updateDisplayedTokenValues();
    updateSwapButtonStateWithBalanceCheck();
    debounceQuoteFetch();
  });

  elements.swapButton.addEventListener("click", async () => {
    if (!walletState.connected) {
      await connectWallet();
      return;
    }

    await executeSwapPlan();
  });

  modalElements.close.addEventListener("click", closeTokenModal);
  modalElements.backdrop.addEventListener("click", closeTokenModal);

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeTokenModal();
    }
  });
}

function bindWalletEvents() {
  if (!window.ethereum) return;

  window.ethereum.on("accountsChanged", async (accounts) => {
    walletState.connected = Array.isArray(accounts) && accounts.length > 0;
    walletState.address = walletState.connected ? accounts[0] : "";

    updateConnectWalletButton();
    updateSwapButtonState();
    clearBalanceCache();

    if (walletState.connected) {
      await refreshDisplayedBalances();
      showHeroView("balance");
    } else {
      resetDisplayedBalances();
      startHeroCycle();
    }

    await fetchQuote();
    updateSwapButtonStateWithBalanceCheck();
  });

  window.ethereum.on("chainChanged", async (chainId) => {
    walletState.chainId = chainId || "";
    clearBalanceCache();

    if (walletState.connected) {
      await refreshDisplayedBalances();
    }

    await fetchQuote();
    updateSwapButtonStateWithBalanceCheck();
  });
}

async function init() {
  bindEvents();
  bindWalletEvents();

  if (window.ethereum) {
    await syncWalletStateFromProvider();
  } else {
    updateConnectWalletButton();
    resetDisplayedBalances();
    updateSwapButtonStateWithBalanceCheck();
  }

  await fetchPrices();
  await loadTokens();
  startHeroCycle();

  startQuoteRefreshLoop();
  startPricesRefreshLoop();
}

init().catch((error) => {
  console.error("failed to initialize app:", error);
  setButtonOverride("Unavailable");
  updateSwapButtonStateWithBalanceCheck();
});