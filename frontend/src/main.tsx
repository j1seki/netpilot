import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import './index.css';

if (location.protocol === 'wails:' || location.hostname === 'wails.localhost') {
  const runtime = '/wails/runtime.js';
  void import(/* @vite-ignore */ runtime).catch(console.error);
}

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
