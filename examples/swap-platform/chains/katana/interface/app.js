const NATIVE_TOKEN_PLACEHOLDERS = new Set([
  "0x0000000000000000000000000000000000000000",
  "native",
]);

const QUOTE_DEBOUNCE_MS = 200;
const QUOTE_REFRESH_MS = 200;

const elements = {
  tokenIn: document.getElementById("tokenIn"),
  tokenOut: document.getElementById("tokenOut"),
  amountIn: document.getElementById("amountIn"),
  amountOut: document.getElementById("amountOut"),
  balanceIn: document.getElementById("balanceIn"),
  balanceOut: document.getElementById("balanceOut"),
  status: document.getElementById("status"),
  flipButton: document.getElementById("flipButton"),
  swapButton: document.getElementById("swapButton"),
  connectWalletButton: document.getElementById("connectWalletButton"),
  currentYear: document.getElementById("current-year"),
};

let tokensState = [];
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

let isSwapExecuting = false;

function setStatus(message, tone = "neutral") {
  elements.status.textContent = message;
  elements.status.className = "text-sm font-semibold";

  if (tone === "success") {
    elements.status.classList.add("text-emerald-600");
    return;
  }

  if (tone === "error") {
    elements.status.classList.add("text-red-600");
    return;
  }

  if (tone === "warning") {
    elements.status.classList.add("text-amber-600");
    return;
  }

  elements.status.classList.add("text-slate-900");
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
  return getTokenSymbol(token);
}

function isNativeToken(token) {
  if (!token) return false;

  if (token.isNative === true || token.IsNative === true) {
    return true;
  }

  const address = getTokenAddress(token);
  if (address && address.toLowerCase() === "0x0000000000000000000000000000000000000000") {
    return true;
  }

  const symbol = getTokenSymbol(token).toLowerCase();
  return NATIVE_TOKEN_PLACEHOLDERS.has(symbol);
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

function loadTokenSelectors(tokens) {
  const firstToken = tokens[0];
  const secondToken = tokens[1] || tokens[0];

  const firstValue = firstToken ? getTokenValue(firstToken) : "";
  const secondValue = secondToken ? getTokenValue(secondToken) : "";

  populateTokenSelect(elements.tokenIn, tokens, firstValue);
  populateTokenSelect(elements.tokenOut, tokens, secondValue);

  if (tokens.length > 1 && firstValue === secondValue) {
    elements.tokenOut.selectedIndex = 1;
  }
}

function flipSelectedTokens() {
  const currentIn = elements.tokenIn.value;
  const currentOut = elements.tokenOut.value;

  elements.tokenIn.value = currentOut;
  elements.tokenOut.value = currentIn;
}

function updateConnectWalletButton() {
  if (!elements.connectWalletButton) return;

  if (!walletState.connected) {
    elements.connectWalletButton.textContent = "Connect Wallet";
    return;
  }

  const shortAddress = `${walletState.address.slice(0, 6)}...${walletState.address.slice(-4)}`;
  elements.connectWalletButton.textContent = shortAddress;
}

function updateSwapButtonState() {
  const hasTokens = tokensState.length >= 2;
  const hasWallet = walletState.connected;
  elements.swapButton.disabled = !(hasTokens && hasWallet);
}

function updateSwapButtonStateWithBalanceCheck() {
  if (isSwapExecuting) {
    elements.swapButton.disabled = true;
    return;
  }

  if (!walletState.connected) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Connect Wallet";
    return;
  }

  const tokenIn = getSelectedToken(elements.tokenIn);
  const tokenOut = getSelectedToken(elements.tokenOut);
  const amount = String(elements.amountIn.value || "").trim();

  if (!tokenIn || !tokenOut) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Select Tokens";
    return;
  }

  if (!amount) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Enter Amount";
    return;
  }

  let amountRaw;
  try {
    amountRaw = parseUnits(amount, currentBalanceInDecimals);
  } catch {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Invalid Amount";
    return;
  }

  if (amountRaw <= 0n) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Enter Amount";
    return;
  }

  if (amountRaw > currentBalanceInRaw) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = `Insufficient ${getTokenSymbol(tokenIn)}`;
    return;
  }

  if (!String(elements.amountOut.value || "").trim()) {
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Loading Quote";
    return;
  }

  elements.swapButton.disabled = false;
  elements.swapButton.textContent = "Swap";
}

function resetDisplayedBalances() {
  currentBalanceInRaw = 0n;
  currentBalanceInDecimals = 18;
  elements.balanceIn.textContent = "Balance: -";
  elements.balanceOut.textContent = "Balance: -";
}

function resetQuoteDisplay() {
  lastSettledQuote = "";
  lastQuoteSignature = "";
  elements.amountOut.value = "";
}

function formatUnits(value, decimals = 18) {
  const big = typeof value === "bigint" ? value : BigInt(value);
  const safeDecimals = Number.isFinite(decimals) ? decimals : 18;
  const base = 10n ** BigInt(safeDecimals);
  const whole = big / base;
  const fraction = big % base;

  if (fraction === 0n) {
    return whole.toString();
  }

  let fractionStr = fraction.toString().padStart(safeDecimals, "0");
  fractionStr = fractionStr.replace(/0+$/, "");
  fractionStr = fractionStr.slice(0, 6);

  if (!fractionStr) {
    return whole.toString();
  }

  return `${whole.toString()}.${fractionStr}`;
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

  return BigInt(whole || "0") * 10n ** BigInt(safeDecimals) + BigInt(paddedFraction || "0");
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
    };
  }

  const [rawBalance, decimals] = await Promise.all([
    readERC20Balance(token, walletAddress),
    readERC20Decimals(token),
  ]);

  return {
    rawBalance,
    decimals,
  };
}

function setBalanceText(element, token, formattedBalance) {
  const symbol = getTokenSymbol(token);
  element.textContent = `Balance: ${formattedBalance} ${symbol}`;
}

async function refreshBalanceForSelection(selectEl, outputEl) {
  if (!walletState.connected || !walletState.address) {
    if (selectEl === elements.tokenIn) {
      currentBalanceInRaw = 0n;
      currentBalanceInDecimals = 18;
    }
    outputEl.textContent = "Balance: -";
    return;
  }

  const token = getSelectedToken(selectEl);
  if (!token) {
    if (selectEl === elements.tokenIn) {
      currentBalanceInRaw = 0n;
      currentBalanceInDecimals = 18;
    }
    outputEl.textContent = "Balance: -";
    return;
  }

  const cacheKey = `${walletState.address}:${walletState.chainId}:${getTokenValue(token)}`;
  const cached = balancesCache.get(cacheKey);

  if (cached) {
    if (selectEl === elements.tokenIn) {
      currentBalanceInRaw = cached.rawBalance;
      currentBalanceInDecimals = cached.decimals;
    }
    setBalanceText(outputEl, token, cached.formatted);
    return;
  }

  outputEl.textContent = "Balance: Loading...";

  try {
    const { rawBalance, decimals } = await readTokenBalance(token, walletState.address);
    const formatted = formatUnits(rawBalance, decimals);

    balancesCache.set(cacheKey, {
      rawBalance,
      decimals,
      formatted,
    });

    if (selectEl === elements.tokenIn) {
      currentBalanceInRaw = rawBalance;
      currentBalanceInDecimals = decimals;
    }

    setBalanceText(outputEl, token, formatted);
  } catch (error) {
    console.error("failed to read token balance:", error);

    if (selectEl === elements.tokenIn) {
      currentBalanceInRaw = 0n;
      currentBalanceInDecimals = getTokenDecimals(token);
    }

    outputEl.textContent = "Balance: Unavailable";
  }
}

async function refreshDisplayedBalances() {
  await Promise.all([
    refreshBalanceForSelection(elements.tokenIn, elements.balanceIn),
    refreshBalanceForSelection(elements.tokenOut, elements.balanceOut),
  ]);

  updateSwapButtonStateWithBalanceCheck();
}

function clearBalanceCache() {
  balancesCache = new Map();
}

function getCurrentQuoteContext() {
  const tokenIn = getSelectedToken(elements.tokenIn);
  const tokenOut = getSelectedToken(elements.tokenOut);
  const amountIn = String(elements.amountIn.value || "").trim();

  if (!tokenIn || !tokenOut) {
    return null;
  }

  if (!amountIn || Number(amountIn) <= 0) {
    return null;
  }

  const tokenInValue = getTokenValue(tokenIn);
  const tokenOutValue = getTokenValue(tokenOut);

  if (!tokenInValue || !tokenOutValue) {
    return null;
  }

  return {
    tokenIn,
    tokenOut,
    tokenInValue,
    tokenOutValue,
    amountIn,
  };
}

async function fetchQuote() {
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

  const decimalsIn = getTokenDecimals(context.tokenIn);
  const decimalsOut = getTokenDecimals(context.tokenOut);

  try {
    const amountInRaw = parseUnits(context.amountIn, decimalsIn);

    const params = new URLSearchParams({
      tokenIn: context.tokenInValue,
      tokenOut: context.tokenOutValue,
      amountIn: amountInRaw.toString(),
    });

    const url = `/quote?${params.toString()}`;

    if (!lastSettledQuote) {
      elements.amountOut.value = "Loading...";
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
    const formatted = formatUnits(amountOutRaw, decimalsOut);

    lastSettledQuote = formatted;
    lastQuoteSignature = signature;
    elements.amountOut.value = formatted;
  } catch (error) {
    console.error("quote failed:", error);

    if (lastSettledQuote && lastQuoteSignature === signature) {
      elements.amountOut.value = lastSettledQuote;
    } else {
      elements.amountOut.value = "";
    }
  } finally {
    isQuoteFetching = false;
    updateSwapButtonStateWithBalanceCheck();

    if (pendingQuoteRerun) {
      pendingQuoteRerun = false;
      fetchQuote();
    }
  }
}

function startQuoteRefreshLoop() {
  stopQuoteRefreshLoop();

  quoteIntervalId = window.setInterval(() => {
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

async function loadTokens() {
  setStatus("Loading tokens...");

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
    rebuildTokenMap(tokensState);
    loadTokenSelectors(tokensState);

    setStatus("Ready", "success");
    updateSwapButtonState();

    if (walletState.connected) {
      await refreshDisplayedBalances();
    }

    await fetchQuote();
    updateSwapButtonStateWithBalanceCheck();
  } catch (error) {
    console.error("failed to load tokens:", error);
    setStatus("Failed to load tokens", "error");
    elements.swapButton.disabled = true;
    elements.swapButton.textContent = "Unavailable";
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
    await refreshDisplayedBalances();
    setStatus("Wallet connected", "success");
  } else {
    resetDisplayedBalances();
  }

  updateSwapButtonStateWithBalanceCheck();
}

async function connectWallet() {
  if (!window.ethereum) {
    setStatus("No wallet detected", "error");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  try {
    setStatus("Connecting wallet...", "warning");

    const accounts = await rpcCall("eth_requestAccounts", []);

    if (!accounts || accounts.length === 0) {
      throw new Error("No accounts returned");
    }

    await syncWalletStateFromProvider();
  } catch (error) {
    console.error("wallet connection failed:", error);
    setStatus("Wallet connection failed", "error");
    updateSwapButtonStateWithBalanceCheck();
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

    await new Promise((resolve) => setTimeout(resolve, pollMs));
  }
}

async function executeSwapPlan() {
  if (isSwapExecuting) return;

  const tokenIn = getSelectedToken(elements.tokenIn);
  const tokenOut = getSelectedToken(elements.tokenOut);
  const amountInHuman = String(elements.amountIn.value || "").trim();

  if (!walletState.connected || !walletState.address) {
    setStatus("Connect wallet first", "warning");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (!tokenIn || !tokenOut) {
    setStatus("Select tokens", "warning");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (!amountInHuman) {
    setStatus("Enter amount", "warning");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  let amountInRaw;
  try {
    amountInRaw = parseUnits(amountInHuman, getTokenDecimals(tokenIn));
  } catch (error) {
    console.error("invalid amount for swap:", error);
    setStatus("Invalid amount", "error");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  if (amountInRaw <= 0n) {
    setStatus("Enter amount", "warning");
    updateSwapButtonStateWithBalanceCheck();
    return;
  }

  isSwapExecuting = true;
  elements.swapButton.disabled = true;
  elements.swapButton.textContent = "Building...";

  try {
    const params = new URLSearchParams({
      user: walletState.address,
      receiver: walletState.address,
      tokenIn: getTokenValue(tokenIn),
      tokenOut: getTokenValue(tokenOut),
      amountIn: amountInRaw.toString(),
    });

    setStatus("Building transaction plan...", "warning");

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
      throw new Error(`swap failed with HTTP ${response.status}`);
    }

    const txs = await response.json();

    if (txs.length === 0) {
      console.error("swap response missing txs:", result);
      throw new Error("No transactions returned");
    }

    for (let i = 0; i < txs.length; i++) {
      const tx = txs[i];

      if (!tx?.to || !tx?.data) {
        console.error("invalid tx object:", tx);
        throw new Error(`Invalid tx at index ${i}`);
      }

      const label = tx.kind || tx.label || `Tx ${i + 1}/${txs.length}`;

      elements.swapButton.textContent =
        txs.length > 1 ? `Confirm ${i + 1}/${txs.length}` : "Confirm Swap";
      setStatus(`Awaiting wallet confirmation: ${label}`, "warning");

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

      setStatus(`Pending confirmation: ${txHash.slice(0, 10)}...`, "warning");
      elements.swapButton.textContent =
        txs.length > 1 ? `Pending ${i + 1}/${txs.length}` : "Pending";

      const receipt = await waitForTransactionReceipt(txHash);

      if (receipt.status === "0x0") {
        throw new Error(`Transaction reverted: ${txHash}`);
      }

      clearBalanceCache();
      await refreshDisplayedBalances();
      await fetchQuote();

      setStatus(`Confirmed: ${txHash.slice(0, 10)}...`, "success");
    }

    setStatus("Swap complete", "success");
    elements.swapButton.textContent = "Swap";
  } catch (error) {
    console.error("swap execution failed:", error);

    const errorMessage = String(error?.message || "").toLowerCase();
    if (errorMessage.includes("user rejected") || errorMessage.includes("user denied")) {
      setStatus("Transaction rejected", "warning");
    } else {
      setStatus("Swap failed", "error");
    }
  } finally {
    isSwapExecuting = false;
    updateSwapButtonStateWithBalanceCheck();
  }
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
      setStatus("Account changed", "success");
      await refreshDisplayedBalances();
    } else {
      resetDisplayedBalances();
      setStatus("Wallet disconnected", "warning");
    }

    await fetchQuote();
    updateSwapButtonStateWithBalanceCheck();
  });

  window.ethereum.on("chainChanged", async (chainId) => {
    walletState.chainId = chainId || "";
    clearBalanceCache();

    if (walletState.connected) {
      setStatus("Network changed", "warning");
      await refreshDisplayedBalances();
    }

    await fetchQuote();
    updateSwapButtonStateWithBalanceCheck();
  });
}

function bindEvents() {
  elements.flipButton.addEventListener("click", async () => {
    if (tokensState.length < 2) return;

    flipSelectedTokens();
    resetQuoteDisplay();

    if (walletState.connected) {
      await refreshDisplayedBalances();
    }

    await fetchQuote();
    updateSwapButtonStateWithBalanceCheck();
  });

  elements.connectWalletButton.addEventListener("click", async () => {
    if (walletState.connected) return;
    await connectWallet();
  });

  elements.tokenIn.addEventListener("change", async () => {
    resetQuoteDisplay();

    if (walletState.connected) {
      await refreshBalanceForSelection(elements.tokenIn, elements.balanceIn);
    }

    updateSwapButtonStateWithBalanceCheck();
    await fetchQuote();
  });

  elements.tokenOut.addEventListener("change", async () => {
    resetQuoteDisplay();

    if (walletState.connected) {
      await refreshBalanceForSelection(elements.tokenOut, elements.balanceOut);
    }

    updateSwapButtonStateWithBalanceCheck();
    await fetchQuote();
  });

  elements.amountIn.addEventListener("input", () => {
    resetQuoteDisplay();
    updateSwapButtonStateWithBalanceCheck();
    debounceQuoteFetch();
  });

  elements.swapButton.addEventListener("click", async () => {
    await executeSwapPlan();
  });
}

async function init() {
  if (elements.currentYear) {
    elements.currentYear.textContent = new Date().getFullYear();
  }

  resetDisplayedBalances();
  resetQuoteDisplay();
  updateConnectWalletButton();
  bindEvents();
  bindWalletEvents();
  startQuoteRefreshLoop();

  if (window.ethereum) {
    try {
      await syncWalletStateFromProvider();
    } catch (error) {
      console.error("failed to sync existing wallet session:", error);
    }
  } else {
    updateSwapButtonStateWithBalanceCheck();
  }

  await loadTokens();
}

init();