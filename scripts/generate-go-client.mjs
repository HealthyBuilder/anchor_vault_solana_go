// Render a Go client from the Anchor IDL with Codama, targeting solana-go v2.
//
//   node scripts/generate-go-client.mjs
//
// Pipeline: Anchor IDL --(nodes-from-anchor)--> Codama RootNode --(renderers-go)--> Go code
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { createFromRoot } from 'codama';
import { rootNodeFromAnchor } from '@codama/nodes-from-anchor';
import { renderVisitor } from '@codama/renderers-go';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const idlPath = path.join(root, 'target/idl/blueshift_anchor_vault.json');
const outDir = path.join(root, 'clients/go/generated');

// 1. Anchor IDL -> Codama node tree
const idl = JSON.parse(readFileSync(idlPath, 'utf8'));
const codama = createFromRoot(rootNodeFromAnchor(idl));

// 2. Codama node tree -> Go code (outDir is wiped before rendering)
await codama.accept(renderVisitor(outDir));

// 3. renderers-go currently emits v1 imports (github.com/gagliardetto/solana-go).
//    v2 keeps the same types and API under a new module path, so rewriting the
//    imports is enough. github.com/gagliardetto/binary is still used by v2 and stays.
const V1 = 'github.com/gagliardetto/solana-go';
const V2 = 'github.com/solana-foundation/solana-go/v2';
for (const file of readdirSync(outDir, { recursive: true })) {
  if (!file.endsWith('.go')) continue;
  const p = path.join(outDir, file);
  const src = readFileSync(p, 'utf8');
  writeFileSync(p, src.replaceAll(`"${V1}"`, `"${V2}"`).replaceAll(`"${V1}/`, `"${V2}/`));
}

console.log(`Go client generated in ${path.relative(root, outDir)}`);
