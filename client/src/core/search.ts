/**
 * Search Functionality for Site Mode
 *
 * Provides client-side search across all pages with:
 * - Fuzzy search through titles and content
 * - Keyboard shortcuts (Ctrl+K / Cmd+K)
 * - Search result highlighting
 * - Modal/overlay UI
 */

import './search.css';

export interface SearchEntry {
  title: string;
  path: string;
  content: string;
  section?: string;
}

export interface SearchResult extends SearchEntry {
  score: number;
  titleMatches: boolean;
  contentMatches: boolean;
}

export class SiteSearch {
  public searchIndex: SearchEntry[] = []; // Made public for testing
  private searchModal: HTMLElement | null = null;
  private searchInput: HTMLInputElement | null = null;
  private resultsContainer: HTMLElement | null = null;
  private selectedIndex: number = 0;

  constructor() {
    // Only initialize in site mode
    if (!document.querySelector('.tinkerdown-nav-sidebar')) {
      return;
    }

    this.init();
  }

  private async init() {
    // Load search index
    await this.loadSearchIndex();

    // Create search UI
    this.createSearchModal();

    // Setup keyboard shortcuts
    this.setupKeyboardShortcuts();

    // Add search button to navigation
    this.addSearchButton();
  }

  private async loadSearchIndex() {
    try {
      const response = await fetch('/search-index.json');
      if (!response.ok) {
        // Search index not available (single tutorial mode)
        return;
      }

      // Check content-type to avoid parsing HTML as JSON
      const contentType = response.headers.get('content-type');
      if (!contentType || !contentType.includes('application/json')) {
        // Server returned non-JSON (likely a redirect to home page)
        return;
      }

      this.searchIndex = await response.json();
      console.log(`[Search] Loaded ${this.searchIndex.length} pages`);
    } catch (error) {
      // Silently ignore - search index is optional in single tutorial mode
    }
  }

  private createSearchModal() {
    this.searchModal = document.createElement('div');
    this.searchModal.className = 'search-modal';
    this.searchModal.innerHTML = `
      <div class="search-backdrop"></div>
      <div class="search-container">
        <div class="search-input-wrapper">
          <svg class="search-icon" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            class="search-input"
            placeholder="Search documentation..."
            autocomplete="off"
            spellcheck="false"
          />
          <button class="search-close" aria-label="Close search">
            <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="search-results"></div>
        <div class="search-footer">
          <div class="search-hints">
            <span><kbd>↑</kbd><kbd>↓</kbd> Navigate</span>
            <span><kbd>↵</kbd> Select</span>
            <span><kbd>Esc</kbd> Close</span>
          </div>
        </div>
      </div>
    `;

    document.body.appendChild(this.searchModal);

    // Get references
    this.searchInput = this.searchModal.querySelector('.search-input');
    this.resultsContainer = this.searchModal.querySelector('.search-results');

    // Setup event listeners
    this.searchInput?.addEventListener('input', () => this.handleSearch());
    this.searchInput?.addEventListener('keydown', (e) => this.handleKeyDown(e));

    this.searchModal.querySelector('.search-close')?.addEventListener('click', () => this.closeSearch());
    this.searchModal.querySelector('.search-backdrop')?.addEventListener('click', () => this.closeSearch());
  }

  private addSearchButton() {
    const sidebar = document.querySelector('.tinkerdown-nav-sidebar');
    if (!sidebar) return;

    const existingButton = sidebar.querySelector<HTMLButtonElement>('[data-search-button], .search-button');
    if (existingButton) {
      existingButton.addEventListener('click', () => this.openSearch());
      return;
    }

    const header = sidebar.querySelector('.nav-header');
    if (!header) return;
    const searchButton = document.createElement('button');
    searchButton.className = 'search-button';
    searchButton.type = 'button';
    searchButton.setAttribute('data-search-button', '');
    searchButton.setAttribute('aria-label', 'Search docs');
    searchButton.innerHTML = `
      <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
      </svg>
      <span>Search</span>
      <kbd>⌘K</kbd>
    `;
    searchButton.addEventListener('click', () => this.openSearch());

    header.appendChild(searchButton);
  }

  private setupKeyboardShortcuts() {
    document.addEventListener('keydown', (e) => {
      // Ctrl+K or Cmd+K
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        this.openSearch();
      }

      // Escape to close
      if (e.key === 'Escape' && this.searchModal?.classList.contains('open')) {
        this.closeSearch();
      }
    });
  }

  private openSearch() {
    this.searchModal?.classList.add('open');
    this.searchInput?.focus();
    this.selectedIndex = 0;
  }

  private closeSearch() {
    this.searchModal?.classList.remove('open');
    if (this.searchInput) {
      this.searchInput.value = '';
    }
    if (this.resultsContainer) {
      this.resultsContainer.innerHTML = '';
    }
  }

  private handleSearch() {
    const query = this.searchInput?.value.trim() || '';

    if (query.length === 0) {
      if (this.resultsContainer) {
        this.resultsContainer.innerHTML = '';
      }
      return;
    }

    const results = this.search(query);
    this.renderResults(results);
  }

  private search(query: string): SearchResult[] {
    const lowerQuery = query.toLowerCase();
    const results: SearchResult[] = [];

    for (const entry of this.searchIndex) {
      const lowerTitle = entry.title.toLowerCase();
      const lowerContent = entry.content.toLowerCase();

      const titleMatches = lowerTitle.includes(lowerQuery);
      const contentMatches = lowerContent.includes(lowerQuery);

      if (titleMatches || contentMatches) {
        // Calculate score (title matches are worth more)
        let score = 0;
        if (titleMatches) {
          score += 10;
          // Exact match or starts with gets bonus
          if (lowerTitle === lowerQuery) score += 50;
          else if (lowerTitle.startsWith(lowerQuery)) score += 20;
        }
        if (contentMatches) {
          score += 1;
        }

        results.push({
          ...entry,
          score,
          titleMatches,
          contentMatches,
        });
      }
    }

    // Sort by score (highest first)
    results.sort((a, b) => b.score - a.score);

    // Limit to top 10 results
    return results.slice(0, 10);
  }

  private renderResults(results: SearchResult[]) {
    if (!this.resultsContainer) return;

    if (results.length === 0) {
      this.resultsContainer.innerHTML = '<div class="search-no-results">No results found</div>';
      return;
    }

    this.resultsContainer.innerHTML = results.map((result, index) => `
      <a
        href="${result.path}"
        class="search-result ${index === this.selectedIndex ? 'selected' : ''}"
        data-index="${index}"
      >
        <div class="search-result-title">
          ${this.highlightMatch(result.title, this.searchInput?.value || '')}
        </div>
        ${result.section ? `<div class="search-result-section">${result.section}</div>` : ''}
        <div class="search-result-content">
          ${this.highlightMatch(this.truncateContent(result.content), this.searchInput?.value || '')}
        </div>
      </a>
    `).join('');

    // Add click handlers
    this.resultsContainer.querySelectorAll('.search-result').forEach((el) => {
      el.addEventListener('click', () => this.closeSearch());
    });
  }

  private highlightMatch(text: string, query: string): string {
    if (!query) return text;

    const regex = new RegExp(`(${this.escapeRegex(query)})`, 'gi');
    return text.replace(regex, '<mark>$1</mark>');
  }

  private truncateContent(content: string, maxLength: number = 200): string {
    if (content.length <= maxLength) return content;
    return content.substring(0, maxLength) + '...';
  }

  private escapeRegex(str: string): string {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  private handleKeyDown(e: KeyboardEvent) {
    if (!this.resultsContainer) return;

    const results = this.resultsContainer.querySelectorAll('.search-result');

    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        this.closeSearch();
        break;

      case 'ArrowDown':
        if (results.length === 0) return;
        e.preventDefault();
        this.selectedIndex = Math.min(this.selectedIndex + 1, results.length - 1);
        this.updateSelection();
        break;

      case 'ArrowUp':
        if (results.length === 0) return;
        e.preventDefault();
        this.selectedIndex = Math.max(this.selectedIndex - 1, 0);
        this.updateSelection();
        break;

      case 'Enter':
        if (results.length === 0) return;
        e.preventDefault();
        const selected = results[this.selectedIndex] as HTMLAnchorElement;
        if (selected) {
          window.location.href = selected.href;
          this.closeSearch();
        }
        break;
    }
  }

  private updateSelection() {
    if (!this.resultsContainer) return;

    const results = this.resultsContainer.querySelectorAll('.search-result');
    results.forEach((el, index) => {
      el.classList.toggle('selected', index === this.selectedIndex);
    });

    // Scroll selected result into view
    const selected = results[this.selectedIndex] as HTMLElement;
    if (selected) {
      selected.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }
}
