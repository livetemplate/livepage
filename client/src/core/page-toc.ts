/**
 * Page Table of Contents (TOC)
 *
 * Adds on-page navigation for H2 sections in documentation pages.
 * This is specifically for site-mode pages that already have site navigation sidebar.
 * Unlike TutorialNavigation (which creates its own sidebar), PageTOC adds a
 * section to the existing sidebar showing H2 sections within the current page.
 */

import './page-toc.css';

export interface PageSection {
  id: string;
  title: string;
  element: HTMLElement;
  index: number;
  level: number;
}

export class PageTOC {
  private sections: PageSection[] = [];
  private tocElement: HTMLElement | null = null;
  private currentActiveSection: number = -1;

  constructor() {
    this.init();
  }

  private init() {
    // Only run in site mode (when site navigation sidebar exists)
    const siteNav = document.querySelector('.tinkerdown-nav-sidebar');
    if (!siteNav) {
      return; // Not in site mode
    }

    // Parse H2 headings as page sections
    this.parseSections();

    if (this.sections.length === 0) {
      return; // No sections to show
    }

    // Create TOC UI in the sidebar
    this.createTOC();

    // Set up scroll tracking to highlight current section
    this.setupScrollTracking();
  }

  private parseSections() {
    // Include H3 entries only for long reference-like pages. Short pages
    // stay H2-only so the sidebar doesn't become noisier than the content.
    const h2s = document.querySelectorAll('.content-wrapper h2');
    const includeH3 = h2s.length >= 8 || document.querySelectorAll('.content-wrapper h3').length >= 10;
    const headings = document.querySelectorAll(includeH3 ? '.content-wrapper h2, .content-wrapper h3' : '.content-wrapper h2');

    headings.forEach((heading, index) => {
      const id = heading.id || this.generateId(heading.textContent || '');
      const tagName = heading.tagName.toLowerCase();

      // Ensure heading has an ID for navigation
      if (!heading.id) {
        heading.id = id;
      }

      this.sections.push({
        id,
        title: heading.textContent || '',
        element: heading as HTMLElement,
        index,
        level: tagName === 'h3' ? 3 : 2
      });
    });
  }

  private generateId(text: string): string {
    return text
      .toLowerCase()
      .replace(/[^\w\s-]/g, '')
      .replace(/\s+/g, '-')
      .replace(/-+/g, '-')
      .trim();
  }

  private createTOC() {
    const siteNav = document.querySelector('.tinkerdown-nav-sidebar');
    if (!siteNav) return;

    // Find the current page link in the navigation
    const currentPageLink = siteNav.querySelector('.nav-page-link.active');
    if (!currentPageLink) return; // Can't add sub-nav if we don't know current page

    const currentPageItem = currentPageLink.closest('li');
    if (!currentPageItem) return;

    // Create nested list of H2 sections
    const subNav = document.createElement('ul');
    subNav.className = 'page-toc-list';
    subNav.innerHTML = this.sections.map((section, i) => `
      <li class="page-toc-item page-toc-level-${section.level}" data-section="${i}">
        <a href="#${section.id}" class="page-toc-link">
          ${this.escapeHtml(section.title)}
        </a>
      </li>
    `).join('');

    // Add click handlers
    subNav.querySelectorAll('.page-toc-item').forEach((itemEl, i) => {
      const link = itemEl.querySelector('a');
      link?.addEventListener('click', (e) => {
        e.preventDefault();
        this.scrollToSection(i);
      });
    });

    // Insert sub-navigation under the current page
    currentPageItem.appendChild(subNav);

    // Add expanded class to current page item
    currentPageItem.classList.add('has-subnav');

    this.tocElement = subNav;
  }

  private escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  private setupScrollTracking() {
    // Track which section is currently visible
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            const heading = entry.target as HTMLElement;
            const sectionIndex = this.sections.findIndex(s => s.element === heading);
            if (sectionIndex !== -1 && sectionIndex !== this.currentActiveSection) {
              this.updateActiveSection(sectionIndex);
            }
          }
        });
      },
      {
        threshold: 0.5,
        rootMargin: '-100px 0px -50% 0px'
      }
    );

    this.sections.forEach(section => observer.observe(section.element));
  }

  private updateActiveSection(index: number) {
    this.currentActiveSection = index;

    if (!this.tocElement) return;

    // Update active state in TOC
    this.tocElement.querySelectorAll('.page-toc-item').forEach((el, i) => {
      el.classList.toggle('active', i === index);
    });
  }

  private scrollToSection(index: number) {
    const section = this.sections[index];
    if (!section) return;

    section.element.scrollIntoView({
      behavior: 'smooth',
      block: 'start'
    });

    // Update URL hash
    history.pushState(null, '', `#${section.id}`);
  }
}
