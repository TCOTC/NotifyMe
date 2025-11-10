import { defineConfig } from 'vite';
import { resolve } from 'path';
import { copyFileSync, mkdirSync, existsSync, readdirSync, statSync } from 'fs';
import { join } from 'path';

// 复制目录的辅助函数
function copyDir(src, dest) {
  if (!existsSync(src)) return;
  
  if (!existsSync(dest)) {
    mkdirSync(dest, { recursive: true });
  }
  
  const entries = readdirSync(src, { withFileTypes: true });
  
  for (const entry of entries) {
    const srcPath = join(src, entry.name);
    const destPath = join(dest, entry.name);
    
    if (entry.isDirectory()) {
      copyDir(srcPath, destPath);
    } else {
      copyFileSync(srcPath, destPath);
    }
  }
}

export default defineConfig({
  build: {
    outDir: 'dist',
    rollupOptions: {
      input: {
        main: resolve(__dirname, 'index.html')
      }
    },
    // 不复制 public 目录，我们手动处理
    copyPublicDir: false
  },
  publicDir: false,
  plugins: [
    {
      name: 'copy-wailsjs-and-src',
      closeBundle() {
        const distDir = resolve(__dirname, 'dist');
        const wailsjsSrc = resolve(__dirname, 'wailsjs');
        const wailsjsDest = resolve(__dirname, 'dist', 'wailsjs');
        const srcDir = resolve(__dirname, 'src');
        const srcDest = resolve(__dirname, 'dist', 'src');
        
        // 复制 wailsjs 目录
        if (existsSync(wailsjsSrc)) {
          console.log('复制 wailsjs 目录...');
          copyDir(wailsjsSrc, wailsjsDest);
        }
        
        // 复制 src 目录（但不包括已经被 Vite 处理的文件）
        if (existsSync(srcDir)) {
          console.log('复制 src 目录...');
          // 只复制 main.js，因为 CSS 已经被 Vite 处理了
          const mainJsSrc = join(srcDir, 'main.js');
          const mainJsDest = join(srcDest, 'main.js');
          if (existsSync(mainJsSrc)) {
            if (!existsSync(srcDest)) {
              mkdirSync(srcDest, { recursive: true });
            }
            copyFileSync(mainJsSrc, mainJsDest);
          }
        }
      }
    }
  ]
});
