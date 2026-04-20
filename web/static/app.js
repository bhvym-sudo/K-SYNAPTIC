class FileManager {
	constructor() {
		this.currentPath = '';
		this.selectedFile = null;
		this.allRoots = [];
		this.init();
	}

	init() {
		this.setupEventListeners();
		this.loadRoots();
		this.loadFiles(this.currentPath);
	}

	setupEventListeners() {
		document.getElementById('uploadBtn').addEventListener('click', () => this.handleUploadClick());
		document.getElementById('newFolderBtn').addEventListener('click', () => this.handleNewFolder());
		
		const uploadZone = document.getElementById('uploadZone');
		const fileList = document.getElementById('fileList');
		
		['dragenter', 'dragover', 'dragleave', 'drop'].forEach(eventName => {
			uploadZone.addEventListener(eventName, (e) => this.preventDefaults(e));
			fileList.addEventListener(eventName, (e) => this.preventDefaults(e));
		});

		['dragenter', 'dragover'].forEach(eventName => {
			uploadZone.addEventListener(eventName, () => uploadZone.classList.add('active'));
			fileList.addEventListener(eventName, () => uploadZone.classList.add('active'));
		});

		['dragleave', 'drop'].forEach(eventName => {
			uploadZone.addEventListener(eventName, () => uploadZone.classList.remove('active'));
			fileList.addEventListener(eventName, () => uploadZone.classList.remove('active'));
		});

		uploadZone.addEventListener('drop', (e) => this.handleDrop(e));
		
		document.getElementById('hiddenUploadInput').addEventListener('change', (e) => this.handleFileSelect(e));
	}

	preventDefaults(e) {
		e.preventDefault();
		e.stopPropagation();
	}

	handleDrop(e) {
		const files = e.dataTransfer.files;
		this.uploadFiles(Array.from(files));
	}

	handleFileSelect(e) {
		const files = e.target.files;
		this.uploadFiles(Array.from(files));
		e.target.value = '';
	}

	handleUploadClick() {
		document.getElementById('hiddenUploadInput').click();
	}

	async uploadFiles(files) {
		for (const file of files) {
			const formData = new FormData();
			formData.append('file', file);
			formData.append('path', this.currentPath);

			try {
				const response = await fetch('/api/upload', {
					method: 'POST',
					body: formData
				});

				if (response.ok) {
					this.showNotification('success', `Uploaded: ${file.name}`);
				} else {
					this.showNotification('error', `Failed to upload: ${file.name}`);
				}
			} catch (error) {
				this.showNotification('error', `Upload error: ${error.message}`);
			}
		}

		this.loadFiles(this.currentPath);
	}

	async loadRoots() {
		try {
			const response = await fetch(`/api/list?path=/`);
			const data = await response.json();
			
			if (data.files) {
				const rootsContainer = document.getElementById('sidebarRoots');
				rootsContainer.innerHTML = '';

				data.files.filter(f => f.is_dir).forEach(file => {
					const item = document.createElement('div');
					item.className = 'root-item';
					item.innerHTML = `<span class="root-icon"></span><span>${file.name || file.path}</span>`;
					item.addEventListener('click', () => this.selectRoot(file.path));
					rootsContainer.appendChild(item);
				});

				if (data.files.length > 0) {
					this.currentPath = data.files[0].path;
					this.updateRootSelection();
					this.loadFiles(this.currentPath);
				}
			}
		} catch (error) {
			console.error('Failed to load roots:', error);
		}
	}

	selectRoot(path) {
		this.currentPath = path;
		this.updateRootSelection();
		this.loadFiles(path);
	}

	updateRootSelection() {
		document.querySelectorAll('.root-item').forEach(item => {
			item.classList.remove('active');
		});
		
		const activeRoot = Array.from(document.querySelectorAll('.root-item')).find(item => {
			return item.textContent.includes(this.currentPath) || 
				   item.textContent.includes(this.currentPath.split('\\').pop()) ||
				   item.textContent.includes(this.currentPath.split('/').pop());
		});
		
		if (activeRoot) {
			activeRoot.classList.add('active');
		}
	}

	async loadFiles(path) {
		document.getElementById('fileList').innerHTML = '<div class="loading">Loading...</div>';

		try {
			const response = await fetch(`/api/list?path=${encodeURIComponent(path)}`);
			const data = await response.json();

			if (data.error) {
				document.getElementById('fileList').innerHTML = `<div class="empty">Error: ${data.error}</div>`;
				return;
			}

			this.currentPath = data.path;
			this.updateBreadcrumb(data.path);
			this.renderFiles(data.files || []);
		} catch (error) {
			document.getElementById('fileList').innerHTML = `<div class="empty">Error loading files</div>`;
		}
	}

	renderFiles(files) {
		const fileList = document.getElementById('fileList');
		
		if (files.length === 0) {
			fileList.innerHTML = '<div class="empty">Empty directory</div>';
			return;
		}

		fileList.innerHTML = '';

		files.sort((a, b) => {
			if (a.is_dir !== b.is_dir) return b.is_dir - a.is_dir;
			return a.name.localeCompare(b.name);
		}).forEach(file => {
			const item = document.createElement('div');
			item.className = 'file-item';
			
			const icon = file.is_dir ? '📁' : this.getFileIcon(file.name);
			const size = file.is_dir ? '' : this.formatSize(file.size);

			item.innerHTML = `
				<div class="file-icon">${icon}</div>
				<div class="file-name" title="${file.name}">${file.name}</div>
				${size ? `<div class="file-size">${size}</div>` : ''}
				<div class="file-actions">
					<button class="file-action-btn" title="Download">⬇</button>
					<button class="file-action-btn" title="Rename">✎</button>
					<button class="file-action-btn" title="Delete">🗑</button>
				</div>
			`;

			if (file.is_dir) {
				item.addEventListener('dblclick', () => this.loadFiles(file.path));
				item.addEventListener('click', () => this.selectFile(item, file.path));
			} else {
				item.addEventListener('click', () => this.selectFile(item, file.path));
			}

			const actions = item.querySelectorAll('.file-action-btn');
			
			if (!file.is_dir) {
				actions[0].addEventListener('click', (e) => {
					e.stopPropagation();
					window.location.href = `/api/download?path=${encodeURIComponent(file.path)}`;
				});
			} else {
				actions[0].style.display = 'none';
			}

			actions[1].addEventListener('click', (e) => {
				e.stopPropagation();
				this.showRenameModal(file.path, file.name);
			});

			actions[2].addEventListener('click', (e) => {
				e.stopPropagation();
				this.deleteFile(file.path);
			});

			fileList.appendChild(item);
		});
	}

	selectFile(element, path) {
		document.querySelectorAll('.file-item').forEach(el => {
			el.classList.remove('selected');
		});
		element.classList.add('selected');
		this.selectedFile = path;
	}

	updateBreadcrumb(path) {
		const breadcrumb = document.getElementById('breadcrumb');
		breadcrumb.innerHTML = '';

		const parts = path.split(/[\\\/]/).filter(p => p);
		
		const homeLink = document.createElement('span');
		homeLink.className = 'breadcrumb-item active';
		homeLink.textContent = 'Home';
		homeLink.addEventListener('click', () => this.loadFiles(this.currentPath));
		breadcrumb.appendChild(homeLink);

		if (parts.length > 0) {
			let fullPath = '';
			parts.forEach((part, index) => {
				fullPath = path.substring(0, path.indexOf(part) + part.length);

				const separator = document.createElement('span');
				separator.className = 'breadcrumb-separator';
				separator.textContent = '/';
				breadcrumb.appendChild(separator);

				const link = document.createElement('span');
				link.className = 'breadcrumb-item';
				link.textContent = part;
				link.addEventListener('click', () => this.loadFiles(fullPath));
				breadcrumb.appendChild(link);
			});
		}
	}

	async deleteFile(path) {
		if (!confirm(`Delete ${path.split(/[\\\/]/).pop()}?`)) return;

		const formData = new FormData();
		formData.append('path', path);

		try {
			const response = await fetch('/api/delete', {
				method: 'POST',
				body: formData
			});

			if (response.ok) {
				this.showNotification('success', 'Deleted successfully');
				this.loadFiles(this.currentPath);
			} else {
				this.showNotification('error', 'Delete failed');
			}
		} catch (error) {
			this.showNotification('error', `Error: ${error.message}`);
		}
	}

	showRenameModal(path, oldName) {
		const modal = document.createElement('div');
		modal.className = 'modal active';
		modal.innerHTML = `
			<div class="modal-content">
				<div class="modal-title">Rename</div>
				<input type="text" class="modal-input" value="${oldName}" id="renameInput" autofocus>
				<div class="modal-buttons">
					<button class="modal-btn modal-btn-secondary" onclick="this.parentElement.parentElement.parentElement.remove()">Cancel</button>
					<button class="modal-btn modal-btn-primary">Rename</button>
				</div>
			</div>
		`;

		const input = modal.querySelector('#renameInput');
		const renameBtn = modal.querySelector('.modal-btn-primary');

		renameBtn.addEventListener('click', () => {
			const newName = input.value.trim();
			if (newName && newName !== oldName) {
				this.renameFile(path, newName);
			}
			modal.remove();
		});

		input.addEventListener('keypress', (e) => {
			if (e.key === 'Enter') renameBtn.click();
			if (e.key === 'Escape') modal.remove();
		});

		document.body.appendChild(modal);
	}

	async renameFile(path, newName) {
		const formData = new FormData();
		formData.append('old_path', path);
		formData.append('new_name', newName);

		try {
			const response = await fetch('/api/rename', {
				method: 'POST',
				body: formData
			});

			if (response.ok) {
				this.showNotification('success', 'Renamed successfully');
				this.loadFiles(this.currentPath);
			} else {
				this.showNotification('error', 'Rename failed');
			}
		} catch (error) {
			this.showNotification('error', `Error: ${error.message}`);
		}
	}

	handleNewFolder() {
		const modal = document.createElement('div');
		modal.className = 'modal active';
		modal.innerHTML = `
			<div class="modal-content">
				<div class="modal-title">New Folder</div>
				<input type="text" class="modal-input" placeholder="Folder name" id="folderInput" autofocus>
				<div class="modal-buttons">
					<button class="modal-btn modal-btn-secondary" onclick="this.parentElement.parentElement.parentElement.remove()">Cancel</button>
					<button class="modal-btn modal-btn-primary">Create</button>
				</div>
			</div>
		`;

		const input = modal.querySelector('#folderInput');
		const createBtn = modal.querySelector('.modal-btn-primary');

		createBtn.addEventListener('click', () => {
			const folderName = input.value.trim();
			if (folderName) {
				this.createFolder(folderName);
			}
			modal.remove();
		});

		input.addEventListener('keypress', (e) => {
			if (e.key === 'Enter') createBtn.click();
			if (e.key === 'Escape') modal.remove();
		});

		document.body.appendChild(modal);
	}

	async createFolder(name) {
		const path = this.currentPath + '/' + name;
		const formData = new FormData();
		formData.append('path', path);

		try {
			const response = await fetch('/api/mkdir', {
				method: 'POST',
				body: formData
			});

			if (response.ok) {
				this.showNotification('success', 'Folder created');
				this.loadFiles(this.currentPath);
			} else {
				this.showNotification('error', 'Failed to create folder');
			}
		} catch (error) {
			this.showNotification('error', `Error: ${error.message}`);
		}
	}

	showNotification(type, message) {
		const notification = document.createElement('div');
		notification.className = `notification ${type}`;
		notification.textContent = message;
		document.body.appendChild(notification);

		setTimeout(() => {
			notification.style.animation = 'slideInRight 0.3s ease reverse';
			setTimeout(() => notification.remove(), 300);
		}, 3000);
	}

	formatSize(bytes) {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
	}

	getFileIcon(filename) {
		const ext = filename.split('.').pop().toLowerCase();
		const icons = {
			'pdf': '📄',
			'doc': '📄', 'docx': '📄', 'txt': '📄',
			'xls': '📊', 'xlsx': '📊', 'csv': '📊',
			'zip': '📦', 'rar': '📦', '7z': '📦',
			'jpg': '🖼️', 'jpeg': '🖼️', 'png': '🖼️', 'gif': '🖼️', 'bmp': '🖼️',
			'mp3': '🎵', 'wav': '🎵', 'flac': '🎵', 'm4a': '🎵',
			'mp4': '🎬', 'avi': '🎬', 'mkv': '🎬', 'mov': '🎬',
			'exe': '⚙️', 'msi': '⚙️', 'app': '⚙️',
			'html': '🌐', 'css': '🌐', 'js': '🌐', 'json': '🌐',
			'py': '🐍', 'go': '🐹', 'rs': '🦀', 'cpp': '⚡', 'c': '⚡'
		};
		return icons[ext] || '📃';
	}
}

document.addEventListener('DOMContentLoaded', () => {
	new FileManager();
});
