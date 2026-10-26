// 用 Codama 把 Anchor IDL 渲染成 Go 客户端，并切换到 solana-go v2。
//
//   node scripts/generate-go-client.mjs
//
// 流程：Anchor IDL --(nodes-from-anchor)--> Codama RootNode --(renderers-go)--> Go 代码
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { createFromRoot } from 'codama';
import { rootNodeFromAnchor } from '@codama/nodes-from-anchor';
import { renderVisitor } from '@codama/renderers-go';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const idlPath = path.join(root, 'target/idl/blueshift_anchor_vault.json');
const outDir = path.join(root, 'clients/go/generated');

// 1. Anchor IDL -> Codama 节点树
const idl = JSON.parse(readFileSync(idlPath, 'utf8'));
const codama = createFromRoot(rootNodeFromAnchor(idl));

// 2. Codama 节点树 -> Go 代码（会先清空 outDir 再生成）
await codama.accept(renderVisitor(outDir));

// 3. renderers-go 目前生成的是 v1 import（github.com/gagliardetto/solana-go），
//    v2 的类型和 API 与 v1 一致，只是模块路径变了，所以直接改写 import 即可。
//    github.com/gagliardetto/binary 在 v2 中仍在使用，保持不变。
const V1 = 'github.com/gagliardetto/solana-go';
const V2 = 'github.com/solana-foundation/solana-go/v2';
for (const file of readdirSync(outDir, { recursive: true })) {
  if (!file.endsWith('.go')) continue;
  const p = path.join(outDir, file);
  const src = readFileSync(p, 'utf8');
  writeFileSync(p, src.replaceAll(`"${V1}"`, `"${V2}"`).replaceAll(`"${V1}/`, `"${V2}/`));
}

console.log(`Go client generated in ${path.relative(root, outDir)}`);
