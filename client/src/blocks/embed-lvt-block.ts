/**
 * EmbedLvtBlock - Wraps a deployed-elsewhere LiveTemplate app inlined
 * into a tinkerdown markdown page. Each embed holds its own
 * LiveTemplateClient pointed at the remote app's WebSocket; tinkerdown's
 * shared MessageRouter is bypassed entirely (the remote app is a
 * separate session, not a tinkerdown-managed block).
 *
 * The container HTML is produced server-side by ProcessEmbedLvt, which
 * already inlined the upstream `<div data-lvt-id="...">` wrapper inside
 * this element. We only have to point a LiveTemplateClient at the right
 * URLs and let it find the wrapper.
 */

import { LiveTemplateClient } from "@livetemplate/client";
import { BaseBlock } from "./base-block";
import { BlockConfig } from "../types";
import { PersistenceManager } from "../core/persistence-manager";

export class EmbedLvtBlock extends BaseBlock {
  private client: LiveTemplateClient | null = null;
  private mountId: string = "";

  constructor(config: BlockConfig, persistence: PersistenceManager, debug = false) {
    super(config, persistence, debug);
  }

  initialize(): void {
    this.log("Initializing embed-lvt block");

    // Server-side ProcessEmbedLvt renames the upstream wrapper's
    // data-lvt-id to data-lvt-id-pending so LiveTemplateClient's
    // module-level autoInit (which scans for [data-lvt-id] on
    // DOMContentLoaded and tries to connect with default URLs) cannot
    // race with us. Rename it back here, before our own connect call.
    //
    // Critically: rename to a unique-per-block id, not the original.
    // LiveTemplate generates one wrapperID per *template*, not per
    // session — so two embeds of the same upstream would otherwise
    // share a data-lvt-id and collide on the LiveTemplate event
    // delegator's per-id key. The collision silently shadows one
    // delegator with the other, leaving one of the regions inert
    // on clicks. Disambiguating by block id keeps each delegator
    // independent.
    const pending = this.element.querySelector<HTMLElement>(
      "[data-lvt-id-pending]",
    );
    if (pending) {
      const original = pending.getAttribute("data-lvt-id-pending");
      if (original) {
        pending.setAttribute("data-lvt-id", `${original}-${this.id}`);
        pending.removeAttribute("data-lvt-id-pending");
      }
    }

    const inner = this.element.querySelector<HTMLElement>("[data-lvt-id]");
    if (!inner) {
      this.error(
        "no [data-lvt-id] inside embed container — server-side fetch likely failed",
      );
      this.element.classList.add("unavailable");
      return;
    }

    // Tag the container with a unique id so the LiveTemplateClient's
    // selector-based connect() can scope to this exact wrapper. Two
    // embeds of the same upstream app on one page would otherwise share
    // a data-lvt-id (livetemplate generates one wrapperID per template,
    // not per visitor) and collide.
    this.mountId = `tinkerdown-embed-mount-${this.id}`;
    this.element.id = this.mountId;
    inner.dataset.tinkerdownEmbedMount = this.id;

    const server = this.element.dataset.embedServer || "";
    const path = this.element.dataset.embedPath || "/";

    const { wsUrl, liveUrl } = this.computeEndpoints(server, path);
    this.log("Connecting to upstream", { wsUrl, liveUrl });

    this.client = new LiveTemplateClient({
      wsUrl,
      liveUrl,
      debug: this.debug,
    });

    // BaseBlock.initialize is synchronous; the real connect handshake
    // is async, so we fire it without await. Failures land in the
    // catch handler and surface as the unavailable badge.
    this.client
      .connect(`#${this.mountId} [data-lvt-id]`)
      .then(() => this.log("Embed connected"))
      .catch((err) => {
        this.error("Embed connect failed:", err);
        this.element.classList.add("unavailable");
      });
  }

  destroy(): void {
    this.log("Destroying embed-lvt block");
    if (this.client) {
      this.client.disconnect();
      this.client = null;
    }
  }

  // EmbedLvtBlock does not participate in tinkerdown's per-block message
  // routing. The remote LiveTemplateClient owns its own WebSocket and
  // dispatches its own actions. handleMessage is here to satisfy the
  // BaseBlock interface contract.
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  handleMessage(_action: string, _data: any, _execMeta: any, _cacheMeta: any): void {
    // intentionally no-op
  }

  /**
   * Compute the upstream HTTP and WebSocket URLs from the embed
   * attributes. The convention LiveTemplate apps follow is "the same
   * mount path serves both HTTP GET (returning HTML) and the WebSocket
   * upgrade" — see `livetemplate/mount.go`. We mirror that: WS URL has
   * the same path as the HTTP URL, just with ws:// (or wss://) scheme.
   *
   * Empty `server` means same-origin: build URLs against the docs
   * page's host so the embed reaches whichever host (and proxy route)
   * the docs page is being served from. `https:` upgrades to `wss:`
   * automatically.
   */
  private computeEndpoints(
    server: string,
    path: string,
  ): { wsUrl: string; liveUrl: string } {
    if (!server) {
      const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
      return {
        wsUrl: `${proto}//${window.location.host}${path}`,
        liveUrl: path,
      };
    }
    const url = new URL(server);
    const wsScheme = url.protocol === "https:" ? "wss:" : "ws:";
    return {
      wsUrl: `${wsScheme}//${url.host}${path}`,
      liveUrl: `${url.origin}${path}`,
    };
  }
}
