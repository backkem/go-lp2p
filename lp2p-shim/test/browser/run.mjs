// Headless browser tests against the checked-in bundle and the demo pages.
// Needs Chromium: /usr/bin/chromium or $CHROMIUM. Skips (exit 0) without it.
//
//   1. iframes.html: postMessage transport with the DOM UI, clicking through
//      the permission prompts, the picker and PIN pairing.
//   2. Two tabs, WebRTC transport, BroadcastChannel signaling.
//   3. Two isolated browser contexts, WebRTC with ManualSignaler: the test
//      plays the human copying codes between devices.
//   4. transport-iframes.html: LP2PQuicTransport and its listener over
//      postMessage, with the DOM UI: stream echo, datagrams, 1 MiB upload.
//   5. transport.html in two tabs over WebRTC: the same, with datagrams on
//      an unreliable RTCDataChannel.
import { existsSync } from "node:fs";
import { createRequire } from "node:module";
import { serve } from "../../scripts/serve.mjs";

const executablePath = process.env.CHROMIUM ?? "/usr/bin/chromium";
if (!existsSync(executablePath)) {
  console.log(`SKIP: no Chromium at ${executablePath} (set CHROMIUM)`);
  process.exit(0);
}
const puppeteer = createRequire(import.meta.url)("puppeteer-core");

const server = await serve(0);
const base = `http://127.0.0.1:${server.address().port}/demo/`;
const browser = await puppeteer.launch({ executablePath, headless: true, args: ["--no-sandbox", "--disable-gpu"] });

const results = [];
async function run(name, fn) {
  if (process.env.ONLY && !name.includes(process.env.ONLY)) return;
  const t0 = Date.now();
  try {
    await fn();
    results.push(true);
    console.log(`ok   ${name} (${Date.now() - t0} ms)`);
  } catch (err) {
    results.push(false);
    console.log(`FAIL ${name} (${Date.now() - t0} ms): ${err.stack ?? err}`);
  }
}

const TIMEOUT = 15_000;

/** Clicks the button labeled text in the shim's dialog inside frame. */
async function clickDialog(frame, text) {
  await frame.waitForFunction(
    (text) => {
      for (const host of document.querySelectorAll("lp2p-shim-dialog")) {
        for (const b of host.shadowRoot.querySelectorAll("button")) {
          if (b.textContent.includes(text)) {
            b.click();
            return true;
          }
        }
      }
      return false;
    },
    { timeout: TIMEOUT, polling: 100 },
    text,
  );
}

async function dialogText(frame, selector) {
  const handle = await frame.waitForFunction(
    (selector) => {
      for (const host of document.querySelectorAll("lp2p-shim-dialog")) {
        const el = host.shadowRoot.querySelector(selector);
        if (el) return el.textContent;
      }
      return false;
    },
    { timeout: TIMEOUT, polling: 100 },
    selector,
  );
  return handle.jsonValue();
}

async function connected(frame) {
  await frame.waitForFunction(() => document.body.dataset.status?.startsWith("Connected"), { timeout: TIMEOUT, polling: 100 });
}

async function chat(from, to, text) {
  // Input events only reach the foreground tab.
  if (typeof from.bringToFront === "function") await from.bringToFront();
  await from.type("#text", text);
  await from.click("#send button");
  await to.waitForFunction((text) => document.getElementById("log").textContent.includes(text), { timeout: TIMEOUT, polling: 100 }, text);
}

function logErrors(page, label) {
  if (process.env.DEBUG) page.on("console", (m) => console.log(`  [${label}] ${m.text()}`));
  page.on("pageerror", (err) => console.log(`  [${label}] page error: ${err.message}`));
}

await run("postMessage between two iframes, DOM UI and PIN pairing", async () => {
  const page = await browser.newPage();
  logErrors(page, "iframes");
  await page.goto(base + "iframes.html");
  const frameOf = async (id) => (await page.$(`#${id}`)).contentFrame();
  const a = await frameOf("a");
  const b = await frameOf("b");
  await a.waitForFunction(() => globalThis.LP2PReceiver && LP2PShim.installed.includes("LP2PReceiver"), { timeout: TIMEOUT, polling: 100 });
  await clickDialog(a, "Allow"); // advertise
  await a.waitForFunction(() => document.body.dataset.status?.startsWith("Discoverable"), { timeout: TIMEOUT, polling: 100 });
  await b.click("#connect");
  await clickDialog(b, "Living room TV"); // picker
  const pin = await dialogText(a, ".pin");
  if (!/^\d{3}-\d{3}-\d{3}$/.test(pin)) throw new Error(`unexpected PIN ${pin}`);
  await b.waitForFunction(() => document.querySelector("lp2p-shim-dialog")?.shadowRoot.querySelector("input"), { timeout: TIMEOUT, polling: 100 });
  await b.evaluate((pin) => {
    document.querySelector("lp2p-shim-dialog").shadowRoot.querySelector("input").value = pin;
  }, pin);
  await clickDialog(b, "Pair");
  await clickDialog(a, "Allow"); // grant the peer, after the ceremony
  await Promise.all([connected(a), connected(b)]);
  await chat(b, a, "hello from the phone");
  await chat(a, b, "hello from the TV");
  // The PIN dialog is gone once pairing is done.
  await a.waitForFunction(() => !document.querySelector("lp2p-shim-dialog"), { timeout: TIMEOUT, polling: 100 });
  await page.close();
});

await run("WebRTC between two tabs, BroadcastChannel signaling", async () => {
  const r = await browser.newPage();
  const q = await browser.newPage();
  logErrors(r, "receiver");
  logErrors(q, "requester");
  await r.goto(base + "chat.html?transport=webrtc&auto=1&role=receiver&name=Desk&debug=1");
  await r.waitForFunction(() => document.body.dataset.status?.startsWith("Discoverable"), { timeout: TIMEOUT, polling: 100 });
  await q.goto(base + "chat.html?transport=webrtc&auto=1&role=requester&name=Laptop&debug=1");
  await Promise.all([connected(r), connected(q)]);
  await chat(q, r, "over SCTP");
  await chat(r, q, "and back");
  await q.close();
  await r.waitForFunction(() => document.body.dataset.status === "Disconnected.", { timeout: TIMEOUT, polling: 100 });
  await r.close();
});

await run("WebRTC between isolated contexts, manual copy/paste signaling, PIN pairing", async () => {
  const ctxA = await browser.createBrowserContext();
  const ctxB = await browser.createBrowserContext();
  const r = await ctxA.newPage();
  const q = await ctxB.newPage();
  logErrors(r, "receiver");
  logErrors(q, "requester");
  await r.goto(base + "chat.html?transport=manual&role=receiver&name=Fridge");
  await q.goto(base + "chat.html?transport=manual&name=Tablet");
  const codeOf = async (page, previous) => {
    const h = await page.waitForFunction(
      (prev) => {
        const v = document.getElementById("outgoing").value;
        return v && v !== prev ? v : false;
      },
      { timeout: TIMEOUT, polling: 100 },
      previous,
    );
    return h.jsonValue();
  };
  const paste = async (page, code) => {
    await page.evaluate((code) => (document.getElementById("incoming").value = code), code);
    await page.evaluate(() => document.getElementById("pasteForm").requestSubmit());
  };
  await clickDialog(r, "Allow"); // advertise
  const announce = await codeOf(r, "");
  // The picker is modal, so the receiver's code goes in before Connect.
  await paste(q, announce);
  await q.click("#connect");
  await clickDialog(q, "Fridge"); // picker
  const offer = await codeOf(q, "");
  await paste(r, offer);
  const answer = await codeOf(r, announce);
  await paste(q, answer);
  const pin = await dialogText(r, ".pin");
  await q.waitForFunction(() => document.querySelector("lp2p-shim-dialog")?.shadowRoot.querySelector("input"), { timeout: TIMEOUT, polling: 100 });
  await q.evaluate((pin) => {
    document.querySelector("lp2p-shim-dialog").shadowRoot.querySelector("input").value = pin;
  }, pin);
  await clickDialog(q, "Pair");
  await clickDialog(r, "Allow"); // grant the peer, after the ceremony
  await Promise.all([connected(r), connected(q)]);
  await chat(q, r, "no server involved");
  await ctxA.close();
  await ctxB.close();
});

/** Runs the WebTransport demo's three checks from the requester page. */
async function exerciseTransport(q) {
  const result = (key) =>
    q.waitForFunction((key) => document.body.dataset[key], { timeout: TIMEOUT, polling: 100 }, key).then((h) => h.jsonValue());
  await q.click("#echo");
  if ((await result("echo")) !== "ok") throw new Error("echo mismatch");
  await q.click("#ping");
  const pings = Number(await result("ping"));
  if (pings < 15) throw new Error(`only ${pings}/20 datagrams came back`);
  await q.click("#bulk");
  if ((await result("bulk")) !== "ok") throw new Error("bulk hash mismatch");
}

await run("WebTransport between two iframes, postMessage, DOM UI and PIN pairing", async () => {
  const page = await browser.newPage();
  logErrors(page, "wt-iframes");
  await page.goto(base + "transport-iframes.html");
  const frameOf = async (id) => (await page.$(`#${id}`)).contentFrame();
  const a = await frameOf("a");
  const b = await frameOf("b");
  await a.waitForFunction(() => globalThis.LP2PQuicTransportListener, { timeout: TIMEOUT, polling: 100 });
  await clickDialog(a, "Allow"); // advertise
  await a.waitForFunction(() => document.body.dataset.status?.startsWith("Discoverable"), { timeout: TIMEOUT, polling: 100 });
  await b.click("#connect");
  await clickDialog(b, "Echo server"); // picker
  const pin = await dialogText(a, ".pin");
  await b.waitForFunction(() => document.querySelector("lp2p-shim-dialog")?.shadowRoot.querySelector("input"), { timeout: TIMEOUT, polling: 100 });
  await b.evaluate((pin) => {
    document.querySelector("lp2p-shim-dialog").shadowRoot.querySelector("input").value = pin;
  }, pin);
  await clickDialog(b, "Pair");
  await clickDialog(a, "Allow"); // grant the peer, after the ceremony
  await Promise.all([connected(a), connected(b)]);
  const reliability = await b.evaluate(() => document.body.dataset.status);
  if (!reliability.includes("reliable-only")) throw new Error(`unexpected status ${reliability}`);
  await exerciseTransport(b);
  await b.click("#close");
  await a.waitForFunction(() => document.body.dataset.status === "Disconnected.", { timeout: TIMEOUT, polling: 100 });
  await page.close();
});

await run("WebTransport between two tabs, WebRTC with unreliable datagrams", async () => {
  const r = await browser.newPage();
  const q = await browser.newPage();
  logErrors(r, "wt-receiver");
  logErrors(q, "wt-requester");
  await r.goto(base + "transport.html?transport=webrtc&auto=1&role=receiver&name=Server");
  await r.waitForFunction(() => document.body.dataset.status?.startsWith("Discoverable"), { timeout: TIMEOUT, polling: 100 });
  await q.goto(base + "transport.html?transport=webrtc&auto=1&role=requester&name=Client");
  await Promise.all([connected(r), connected(q)]);
  const status = await q.evaluate(() => document.body.dataset.status);
  if (!status.includes("supports-unreliable")) throw new Error(`unexpected status ${status}`);
  await exerciseTransport(q);
  await q.close();
  await r.waitForFunction(() => document.body.dataset.status === "Disconnected.", { timeout: TIMEOUT, polling: 100 });
  await r.close();
});

await browser.close();
server.close();
const failed = results.filter((ok) => !ok).length;
console.log(`${results.length - failed}/${results.length} browser tests passed`);
process.exit(failed ? 1 : 0);
