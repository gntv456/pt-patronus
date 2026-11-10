// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 导出 PT-depiler 的 286 个站点定义为 JSON，供 convert-definitions.go 翻译为 PTPatronus YAML。
//
// 实现方式：TypeScript AST 提取（非运行时 import）。PT-depiler 的 node_modules 未安装，
// 且 schema 基类 import 链依赖外部 npm 包（sizzle/es-toolkit），运行时 import 不可行。
// 改用 ts 编译器解析每个 .ts，定位 `export const siteMetadata = {...}` 的对象字面量，
// 递归提取显式字段：
//   - 数据字面量（字符串/数字/数组/对象）→ 原值
//   - spread（`...SchemaMetadata`）→ __spread 标记（继承默认；NexusPHP 站本项目 EnsureDefaults 自填）
//   - 函数（elementProcess/匿名 filter/case）→ {__fn, src}（convert 标 TODO 人工翻译）
//   - 外部常量引用（CategoryIncldead 等）→ __ref（NexusPHP 通用过滤，本项目忽略）
//   - 模板串/正则/函数调用 → __tpl/__regex/__call（保留原文供人工）
//
// 运行：
//   cd /d/PTPatronus/backend/tools && node export-definitions.mjs
import * as ts from "typescript";
import { readdirSync, readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { join, basename, extname } from "node:path";

const DEFS_DIR = process.env.PT_DEPILER_DEFS_DIR || "D:/PT-depiler-master/src/packages/site/definitions";
const OUT_DIR = process.env.PT_DEFS_DUMP_DIR || "D:/PTPatronus/backend/tools/dump";
mkdirSync(OUT_DIR, { recursive: true });

// nodeToValue 把 AST 节点转为可 JSON 序列化的值。
function nodeToValue(node) {
  if (!node) return null;
  switch (node.kind) {
    case ts.SyntaxKind.StringLiteral:
      return node.text;
    case ts.SyntaxKind.NumericLiteral:
      return Number(node.text);
    case ts.SyntaxKind.TrueKeyword:
      return true;
    case ts.SyntaxKind.FalseKeyword:
      return false;
    case ts.SyntaxKind.NullKeyword:
      return null;
    case ts.SyntaxKind.NoSubstitutionTemplateLiteral:
      return node.text;
    case ts.SyntaxKind.ArrayLiteralExpression:
      return node.elements.map(nodeToValue);
    case ts.SyntaxKind.ObjectLiteralExpression: {
      const obj = {};
      for (const prop of node.properties) {
        if (ts.isSpreadAssignment(prop)) {
          obj.__spread = (obj.__spread || []).concat(prop.expression.getText());
        } else if (ts.isPropertyAssignment(prop)) {
          obj[prop.name.getText()] = nodeToValue(prop.initializer);
        } else if (ts.isShorthandPropertyAssignment(prop)) {
          obj[prop.name.getText()] = { __ref: prop.name.getText() };
        }
      }
      return obj;
    }
    case ts.SyntaxKind.ArrowFunction:
    case ts.SyntaxKind.FunctionExpression:
      return { __fn: true, src: node.getText() };
    case ts.SyntaxKind.Identifier:
      return { __ref: node.text };
    case ts.SyntaxKind.PropertyAccessExpression:
      return { __ref: node.getText() };
    case ts.SyntaxKind.TemplateExpression:
    case ts.SyntaxKind.TemplateLiteral:
      return { __tpl: node.getText() };
    case ts.SyntaxKind.CallExpression:
    case ts.SyntaxKind.NewExpression:
      return { __call: node.getText() };
    case ts.SyntaxKind.RegularExpressionLiteral:
    case ts.SyntaxKind.RegularExpression:
      return { __regex: node.getText() };
    case ts.SyntaxKind.PrefixUnaryExpression:
      return { __expr: node.getText() };
    default:
      return { __raw: ts.SyntaxKind[node.kind], src: node.getText().slice(0, 200) };
  }
}

// objectLiteralOf 剥离 as/satisfies/括号，取到对象字面量（处理 `{} as ISiteMetadata` 等写法）。
function objectLiteralOf(init) {
  if (!init) return null;
  if (ts.isObjectLiteralExpression(init)) return init;
  if (ts.isAsExpression(init) || ts.isSatisfiesExpression(init) || ts.isParenthesizedExpression(init)) {
    return objectLiteralOf(init.expression);
  }
  return null;
}

// extractSiteMetadata 解析源码，定位 siteMetadata 对象字面量并提取。
function extractSiteMetadata(sourceText) {
  const sf = ts.createSourceFile("x.ts", sourceText, ts.ScriptTarget.Latest, true);
  let result = null;
  const visit = (node) => {
    if (result) return;
    if (ts.isVariableStatement(node)) {
      for (const d of node.declarationList.declarations) {
        if (d.name.getText() === "siteMetadata") {
          const obj = objectLiteralOf(d.initializer);
          if (obj) {
            result = nodeToValue(obj);
            return;
          }
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return result;
}

const entries = readdirSync(DEFS_DIR);
const tsFiles = entries.filter((f) => f.endsWith(".ts")).sort();
let ok = 0;
const errors = [];

for (const f of tsFiles) {
  const id = basename(f, extname(f));
  try {
    const src = readFileSync(join(DEFS_DIR, f), "utf8");
    const meta = extractSiteMetadata(src);
    if (!meta || typeof meta !== "object") {
      errors.push(`${id}: no siteMetadata object literal`);
      continue;
    }
    writeFileSync(join(OUT_DIR, `${id}.json`), JSON.stringify(meta, null, 2));
    ok++;
  } catch (e) {
    errors.push(`${id}: ${e && e.message ? e.message : e}`);
  }
}

writeFileSync(join(OUT_DIR, "_errors.txt"), errors.join("\n") + "\n");
console.log(`extracted ${ok}/${tsFiles.length} definitions → ${OUT_DIR}`);
if (errors.length) console.log(`${errors.length} failed, see _errors.txt`);
