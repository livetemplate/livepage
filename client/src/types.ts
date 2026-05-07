/**
 * Core types for Tinkerdown Client
 */

export type BlockType = "server" | "wasm" | "interactive" | "lvt" | "embed-lvt";

export interface BlockMetadata {
  id: string;
  type: BlockType;
  language: string;
  readonly?: boolean;
  editable?: boolean;
  stateRef?: string; // Reference to server state (for interactive blocks)
}

export interface ExecMeta {
  status: string;
  duration?: number;
  output?: string;
  stderr?: string;
  command?: string;
}

export interface CacheMeta {
  cached: boolean;
  age?: string;
  expires_in?: string;
  stale: boolean;
  refreshing: boolean;
}

export interface MessageEnvelope {
  blockID: string;
  action: string;
  data: any;
  execMeta?: ExecMeta;
  cacheMeta?: CacheMeta;
}

export interface TinkerdownClientOptions {
  wsUrl: string;
  debug?: boolean;
  persistence?: boolean;
  cdnFallback?: boolean;
  onConnect?: () => void;
  onDisconnect?: () => void;
  onError?: (error: Error) => void;
}

export interface BlockConfig {
  element: HTMLElement;
  metadata: BlockMetadata;
  initialCode?: string;
}

export interface WasmExecutionResult {
  stdout: string;
  stderr: string;
  error?: string;
  exitCode: number;
}

export interface EditorOptions {
  language: string;
  readonly: boolean;
  theme?: string;
  minimap?: boolean;
  lineNumbers?: boolean;
}

export interface PersistenceData {
  code: Record<string, string>; // blockID -> code
  timestamp: number;
}
