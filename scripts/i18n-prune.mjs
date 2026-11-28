#!/usr/bin/env node
// Prunes the English dictionary of the console.
//
// The dictionary maps a Chinese source string to its English. It is written as
// a first assignment plus a long tail of Object.assign calls. When a surface is
// removed, its strings stop being rendered but stay in the dictionary, so the
// file grows with entries nothing can ever match. This script keeps the
// entries whose Chinese key still appears in the console and drops the rest.
//
//   node scripts/i18n-prune.mjs            # report what would be dropped
//   node scripts/i18n-prune.mjs --write    # drop them
//
// A key is live when the console's HTML or one of its JavaScript modules
// still contains it. A key that only ever existed in a removed surface does
// not appear anywhere, so it can never be matched again.
import { readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const web = path.join(root, 'web');

async function collectSources() {
  const sources = [await readFile(path.join(web, 'index.html'), 'utf8')];
  const appDir = path.join(web, 'app');
  for (const name of await readdir(appDir)) {
    if (!name.endsWith('.js')) continue;
    sources.push(await readFile(path.join(appDir, name), 'utf8'));
  }
  return sources;
}

// The dictionary is a first assignment plus Object.assign calls. Every one of
// them holds a JSON object; collect them all.
async function readDictionary() {
  const file = path.join(web, 'i18n-en.js');
  const source = await readFile(file, 'utf8');
  const entries = {};
  const pattern = /Object\.assign\(window\.LF_I18N_EN\s*,\s*/g;
  let match;
  while ((match = pattern.exec(source)) !== null) {
    const start = source.indexOf('{', match.index + match[0].length - 1);
    if (start < 0) break;
    // Walk the object by brace depth so a string containing "}" does not end it.
    let depth = 0;
    let inString = false;
    let escaped = false;
    let end = -1;
    for (let i = start; i < source.length; i++) {
      const ch = source[i];
      if (inString) {
        if (escaped) escaped = false;
        else if (ch === '\\') escaped = true;
        else if (ch === '"') inString = false;
        continue;
      }
      if (ch === '"') inString = true;
      else if (ch === '{') depth++;
      else if (ch === '}') {
        depth--;
        if (depth === 0) { end = i; break; }
      }
    }
    if (end < 0) throw new Error('the dictionary has an unbalanced object');
    // One of the assignments ends with a trailing comma, which JSON forbids.
    const object = source.slice(start, end + 1).replace(/,\s*\}$/, '}');
    Object.assign(entries, JSON.parse(object));
    pattern.lastIndex = end + 1;
  }
  return { file, entries };
}

const sources = await collectSources();
const haystack = sources.join('\n');
const { file, entries } = await readDictionary();
const keys = Object.keys(entries);
const dead = keys.filter(key => !haystack.includes(key));
const live = keys.filter(key => haystack.includes(key));

console.log(`dictionary entries: ${keys.length} (live ${live.length}, dead ${dead.length})`);

if (process.argv.includes('--write') && dead.length) {
  const kept = {};
  for (const key of live) kept[key] = entries[key];
  await writeFile(file, `window.LF_I18N_EN=${JSON.stringify(kept)};\n`, 'utf8');
  console.log(`dropped ${dead.length} dead entries`);
} else if (dead.length) {
  console.log('\ndead entries (first 20):');
  for (const key of dead.slice(0, 20)) console.log(`  ${key}`);
}
