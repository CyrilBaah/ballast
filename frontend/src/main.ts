// Frontend entry point. Boots the redesigned UI (src/ui/), which is wired
// to the real Wails/Drive backend via src/ui/live.ts (a thin bridge over
// src/api/*.ts). Journal and Field Lab still run on static mock data in
// src/ui/mock-data.ts -- there's no real per-chunk event log or "break it
// on purpose" endpoint to back them. The old screens (src/screens/) are
// superseded and no longer mounted.
import './ui/tokens.css';
import './ui/base.css';
import './ui/components.css';
import './ui/layout.css';
import './ui/views.css';

import { mountApp } from './ui/shell';

const app = document.querySelector<HTMLElement>('#app')!;
mountApp(app);
