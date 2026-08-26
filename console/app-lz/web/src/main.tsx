/**
 * React 应用入口文件。
 *
 * 启动流程：
 *  1. 创建 React 根节点，挂载到 index.html 中的 #root 元素
 *  2. 包裹 React.StrictMode（开发模式下检测副作用）
 *  3. 包裹 I18nProvider（提供中英文国际化上下文）
 *  4. 渲染 App 根组件（管理所有状态和面板）
 */
import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import { I18nProvider } from './i18n';
import './index.css';  // Tailwind CSS 全局样式

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </React.StrictMode>,
);
