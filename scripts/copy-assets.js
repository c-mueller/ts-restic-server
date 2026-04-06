#!/usr/bin/env node
// Copies Bootswatch (Bootstrap) assets from node_modules to the Go embed directory.
// Run: npm run build:ui

const fs = require('fs');
const path = require('path');

const src = path.join(__dirname, '..', 'node_modules');
const dst = path.join(__dirname, '..', 'internal', 'ui', 'static');

function copy(from, to) {
  fs.mkdirSync(path.dirname(to), { recursive: true });
  fs.copyFileSync(from, to);
  console.log(`  ${path.relative(path.join(__dirname, '..'), from)} -> ${path.relative(path.join(__dirname, '..'), to)}`);
}

console.log('Copying Bootswatch/Bootstrap assets...');
copy(path.join(src, 'bootswatch', 'dist', 'darkly', 'bootstrap.min.css'), path.join(dst, 'css', 'bootstrap.min.css'));
copy(path.join(src, 'bootstrap', 'dist', 'js', 'bootstrap.bundle.min.js'), path.join(dst, 'js', 'bootstrap.bundle.min.js'));
console.log('Done.');
