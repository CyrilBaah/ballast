// File + folder picker screen: pick a local file, browse Drive destination
// folders (defaulting to "My Drive"), and start the upload.
import { PickLocal, type LocalFileRef } from '../api/files';
import { ListFolders, type DriveFolder } from '../api/drive';
import { Start } from '../api/upload';
import { SignOut } from '../api/auth';
import { toPlainLanguage } from '../errors';

export interface PickerScreenOptions {
    container: HTMLElement;
    email?: string;
    onUploadStarted: (uploadId: number, file: LocalFileRef, folder: BreadcrumbEntry) => void;
    onSignedOut: () => void;
}

interface BreadcrumbEntry {
    id: string; // "" represents the Drive root ("My Drive")
    name: string;
}

const ROOT: BreadcrumbEntry = { id: '', name: 'My Drive' };

export function renderPicker(opts: PickerScreenOptions): () => void {
    const { container, email, onUploadStarted, onSignedOut } = opts;

    let selectedFile: LocalFileRef | null = null;
    let path: BreadcrumbEntry[] = [ROOT];
    let folders: DriveFolder[] = [];
    let starting = false;
    let disposed = false;

    container.innerHTML = `
        <div class="picker-screen">
            <h1>Ballast</h1>
            <div class="picker-header">
                <p class="picker-account">Signed in as ${email ?? 'unknown'}.</p>
                <button id="sign-out-btn" class="btn btn-secondary">Sign out</button>
            </div>
            <p id="sign-out-error" class="state-error" role="alert"></p>

            <section class="picker-file">
                <button id="pick-file-btn" class="btn">Choose a file…</button>
                <p id="picked-file" class="picker-file-name"></p>
            </section>

            <section class="picker-folders">
                <p id="picker-breadcrumb" class="picker-breadcrumb"></p>
                <p id="picker-folder-loading" class="state-loading" role="status" aria-live="polite" hidden>
                    <span class="spinner" aria-hidden="true"></span>Loading folders…
                </p>
                <ul id="picker-folder-list" class="picker-folder-list"></ul>
                <p id="picker-folder-error" class="state-error" role="alert"></p>
            </section>

            <button id="upload-btn" class="btn" disabled>Upload</button>
            <p id="upload-error" class="state-error" role="alert"></p>
        </div>
    `;

    const signOutBtn = container.querySelector<HTMLButtonElement>('#sign-out-btn')!;
    const signOutErrorEl = container.querySelector<HTMLParagraphElement>('#sign-out-error')!;
    const pickFileBtn = container.querySelector<HTMLButtonElement>('#pick-file-btn')!;
    const pickedFileEl = container.querySelector<HTMLParagraphElement>('#picked-file')!;
    const breadcrumbEl = container.querySelector<HTMLParagraphElement>('#picker-breadcrumb')!;
    const folderLoadingEl = container.querySelector<HTMLParagraphElement>('#picker-folder-loading')!;
    const folderListEl = container.querySelector<HTMLUListElement>('#picker-folder-list')!;
    const folderErrorEl = container.querySelector<HTMLParagraphElement>('#picker-folder-error')!;
    const uploadBtn = container.querySelector<HTMLButtonElement>('#upload-btn')!;
    const uploadErrorEl = container.querySelector<HTMLParagraphElement>('#upload-error')!;

    function currentFolder(): BreadcrumbEntry {
        return path[path.length - 1];
    }

    function updateUploadButton() {
        uploadBtn.disabled = starting || !selectedFile;
    }

    function renderBreadcrumb() {
        breadcrumbEl.innerHTML = '';
        path.forEach((entry, idx) => {
            const isLast = idx === path.length - 1;
            const span = document.createElement(isLast ? 'strong' : 'a');
            span.textContent = entry.name;
            if (!isLast) {
                (span as HTMLAnchorElement).href = '#';
                span.addEventListener('click', (e) => {
                    e.preventDefault();
                    path = path.slice(0, idx + 1);
                    void loadFolders();
                });
            }
            breadcrumbEl.appendChild(span);
            if (!isLast) breadcrumbEl.appendChild(document.createTextNode(' / '));
        });
    }

    function renderFolderList() {
        folderListEl.innerHTML = '';
        folders.forEach((folder) => {
            const li = document.createElement('li');
            const btn = document.createElement('button');
            btn.className = 'btn picker-folder-item';
            btn.textContent = folder.name + (folder.hasChildren ? ' ›' : '');
            btn.addEventListener('click', () => {
                path = [...path, { id: folder.id, name: folder.name }];
                void loadFolders();
            });
            li.appendChild(btn);
            folderListEl.appendChild(li);
        });
        if (folders.length === 0) {
            const li = document.createElement('li');
            li.className = 'picker-folder-empty';
            li.textContent = 'No sub-folders here.';
            folderListEl.appendChild(li);
        }
    }

    async function loadFolders() {
        renderBreadcrumb();
        folderErrorEl.textContent = '';
        folderLoadingEl.hidden = false;
        folderListEl.innerHTML = '';
        try {
            folders = await ListFolders(currentFolder().id);
            if (disposed) return;
            renderFolderList();
        } catch (err) {
            if (disposed) return;
            folders = [];
            renderFolderList();
            const message = err instanceof Error ? err.message : String(err);
            folderErrorEl.textContent = toPlainLanguage(message);
        } finally {
            if (!disposed) folderLoadingEl.hidden = true;
        }
    }

    signOutBtn.addEventListener('click', async () => {
        signOutBtn.disabled = true;
        signOutErrorEl.textContent = '';
        try {
            await SignOut();
            if (disposed) return;
            onSignedOut();
        } catch (err) {
            if (disposed) return;
            signOutBtn.disabled = false;
            const message = err instanceof Error ? err.message : String(err);
            signOutErrorEl.textContent = toPlainLanguage(message);
        }
    });

    pickFileBtn.addEventListener('click', async () => {
        try {
            const file = await PickLocal();
            if (!file) return; // user cancelled the dialog
            selectedFile = file;
            pickedFileEl.textContent = `${file.name} (${file.sizeBytes} bytes)`;
            updateUploadButton();
        } catch (err) {
            pickedFileEl.textContent = '';
            const message = err instanceof Error ? err.message : String(err);
            uploadErrorEl.textContent = toPlainLanguage(message);
        }
    });

    uploadBtn.addEventListener('click', async () => {
        if (!selectedFile) return;
        uploadErrorEl.textContent = '';
        starting = true;
        updateUploadButton();
        try {
            const uploadId = await Start(selectedFile.path, currentFolder().id || 'root', currentFolder().name);
            onUploadStarted(uploadId, selectedFile, currentFolder());
        } catch (err) {
            starting = false;
            updateUploadButton();
            const message = err instanceof Error ? err.message : String(err);
            uploadErrorEl.textContent = toPlainLanguage(message);
        }
    });

    void loadFolders();

    return () => {
        disposed = true;
    };
}
