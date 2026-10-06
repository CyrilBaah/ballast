// File picker — picks one or more local files via the native OS dialog
// (Files.PickLocalMultiple), then lets the user browse real Drive folders
// (Drive.ListFolders) as one shared upload destination for the whole batch
// before starting. The mock version's virtual "SD card / SSD / laptop"
// filesystem browser is gone -- there's no real API that could back it, and
// Wails' native picker is a system dialog, not something this UI can render
// a custom browser inside of.

import { icon, kindIcon, button } from '../components';
import { formatBytes } from '../format';
import * as live from '../live';
import type { DriveFolder, LocalFileRef } from '../live';
import { toPlainLanguage } from '../../errors';

interface BreadcrumbEntry {
    id: string; // "" represents the Drive root ("My Drive")
    name: string;
}

const ROOT: BreadcrumbEntry = { id: '', name: 'My Drive' };

export function openFilePicker(opts: { onClose: () => void; onStarted: () => void }): void {
    void live
        .pickFiles()
        .then((files) => {
            if (!files.length) {
                opts.onClose(); // user cancelled the native dialog
                return;
            }
            renderDestinationModal(files, opts);
        })
        .catch(() => opts.onClose());
}

function renderDestinationModal(files: LocalFileRef[], opts: { onClose: () => void; onStarted: () => void }): void {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.appendChild(overlay);

    let path: BreadcrumbEntry[] = [ROOT];
    let folders: DriveFolder[] = [];
    let selectedFolder: BreadcrumbEntry = ROOT;
    let loadingFolders = false;
    let folderError = '';
    let disposed = false;
    const totalBytes = files.reduce((sum, f) => sum + f.sizeBytes, 0);

    function close() {
        overlay.remove();
        opts.onClose();
    }

    function currentFolder(): BreadcrumbEntry {
        return path[path.length - 1];
    }

    async function loadFolders() {
        loadingFolders = true;
        folderError = '';
        renderAll();
        try {
            folders = await live.listDriveFolders(currentFolder().id);
        } catch (err) {
            if (disposed) return;
            folders = [];
            const message = err instanceof Error ? err.message : String(err);
            folderError = toPlainLanguage(message);
        } finally {
            loadingFolders = false;
        }
        if (!disposed) renderAll();
    }

    function renderAll() {
        const crumbs = path;
        overlay.innerHTML = `
            <div class="modal-backdrop" id="picker-backdrop"></div>
            <div class="rise picker-modal">
                <div class="picker-modal-head">
                    <div style="display:flex;align-items:center;gap:12px">
                        <span class="picker-modal-icon tone-grass">${icon.folder('icon icon-lg')}</span>
                        <div>
                            <h2 class="display" style="margin:0;font-size:19px;line-height:1.2">Send this to Drive</h2>
                            <p style="margin:2px 0 0;font-size:12px;color:var(--color-mute)">Choose where it lands. Size is no object.</p>
                        </div>
                    </div>
                    ${button(icon.x(), { variant: 'ghost', size: 'icon', attrs: 'id="picker-close"' })}
                </div>

                <div class="picker-modal-body">
                    <section class="picker-browser" style="flex:1">
                        <div class="picker-browser-chrome">
                            <div class="picker-breadcrumb">
                                ${crumbs
                                    .map(
                                        (c, i) => `<span style="display:flex;align-items:center;gap:6px">
                                        ${i > 0 ? icon.chevron('icon') : ''}
                                        <button type="button" class="picker-crumb ${i === crumbs.length - 1 ? 'current' : ''}" data-crumb="${i}">${c.name}</button>
                                    </span>`,
                                    )
                                    .join('')}
                            </div>
                        </div>
                        <div class="scroll picker-file-list" id="picker-folder-list">
                            ${
                                loadingFolders
                                    ? `<p class="state-loading" style="padding:16px;font-size:12.5px;color:var(--color-mute)"><span class="spinner" aria-hidden="true"></span>Loading folders…</p>`
                                    : folderError
                                      ? `<p class="state-error" style="padding:16px;font-size:12.5px">${folderError}</p>`
                                      : renderFolderRows()
                            }
                        </div>
                    </section>

                    <aside class="picker-tray">
                        <div class="picker-tray-head">
                            <p class="picker-side-label">Selected</p>
                            <p class="display tnum picker-tray-total">${formatBytes(totalBytes)}</p>
                            <p style="margin:2px 0 0;font-size:12px;color:var(--color-mute)">${files.length} file${files.length === 1 ? '' : 's'}</p>
                        </div>

                        <div class="scroll picker-tray-list">
                            ${files
                                .map(
                                    (file) => `<div class="picker-tray-item">
                                <span class="picker-file-icon tone-grass" style="flex-shrink:0">${kindIcon(live.fileKind(file.name), 'icon')}</span>
                                <span style="min-width:0;flex:1">
                                    <span style="display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;font-weight:500">${file.name}</span>
                                    <span class="tnum" style="display:block;font-size:10.5px;color:var(--color-faint)">${formatBytes(file.sizeBytes)}</span>
                                </span>
                            </div>`,
                                )
                                .join('')}
                        </div>

                        <div class="picker-tray-footer">
                            <p class="picker-side-label" style="margin-bottom:6px">Sending to</p>
                            <div class="picker-dest-row active" style="cursor:default">
                                <span class="picker-dest-icon tone-grass">${icon.drive('icon')}</span>
                                <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:11.5px;font-weight:500">${selectedFolder.name}</span>
                            </div>

                            ${button(`${icon.bolt()} Start upload`, {
                                variant: 'primary',
                                className: 'picker-start-btn',
                                attrs: `id="picker-start"`,
                            })}
                        </div>
                    </aside>
                </div>
            </div>
        `;

        overlay.querySelector('#picker-backdrop')?.addEventListener('click', close);
        overlay.querySelector('#picker-close')?.addEventListener('click', close);
        overlay.querySelectorAll<HTMLElement>('[data-crumb]').forEach((el) => {
            el.addEventListener('click', () => {
                const idx = Number(el.dataset.crumb);
                path = path.slice(0, idx + 1);
                selectedFolder = currentFolder();
                void loadFolders();
            });
        });
        overlay.querySelectorAll<HTMLElement>('[data-open-folder]').forEach((el) => {
            el.addEventListener('click', () => {
                const folder = folders.find((f) => f.id === el.dataset.openFolder);
                if (!folder) return;
                path = [...path, { id: folder.id, name: folder.name }];
                selectedFolder = currentFolder();
                void loadFolders();
            });
        });
        overlay.querySelector('#picker-start')?.addEventListener('click', () => {
            live.enqueueUploads(files, selectedFolder);
            close();
            opts.onStarted();
        });
    }

    function renderFolderRows(): string {
        if (!folders.length) {
            return `<div class="picker-empty">
                <span class="picker-empty-icon">${icon.folder('icon icon-lg')}</span>
                <p style="margin:12px 0 0;font-size:13px;font-weight:500">No sub-folders here</p>
                <p style="margin:4px 0 0;font-size:12px;color:var(--color-faint)">This folder is already selected as the destination.</p>
            </div>`;
        }
        return folders
            .map(
                (f) => `<button type="button" class="picker-sub-item" data-open-folder="${f.id}">
                    <span class="picker-sub-item-icon tone-butter">${icon.folder('icon icon-lg')}</span>
                    <span style="min-width:0;flex:1;text-align:left">
                        <span style="display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:13px;font-weight:500">${f.name}</span>
                    </span>
                    ${f.hasChildren ? icon.chevron('icon fg-faint') : ''}
                </button>`,
            )
            .join('');
    }

    void loadFolders();
}
