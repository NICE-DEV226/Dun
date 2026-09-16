const revealItems = document.querySelectorAll('.reveal');
const copyButton = document.querySelector('[data-copy]');

if ('IntersectionObserver' in window) {
  const observer = new IntersectionObserver((entries, currentObserver) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) return;
      entry.target.classList.add('visible');
      currentObserver.unobserve(entry.target);
    });
  }, { threshold: 0.12 });

  revealItems.forEach((item) => observer.observe(item));
} else {
  revealItems.forEach((item) => item.classList.add('visible'));
}

copyButton?.addEventListener('click', async () => {
  const command = copyButton.dataset.copy;
  const originalLabel = copyButton.innerHTML;

  try {
    await navigator.clipboard.writeText(command);
    copyButton.textContent = 'Copied';
  } catch {
    copyButton.textContent = 'Copy unavailable';
  }

  window.setTimeout(() => {
    copyButton.innerHTML = originalLabel;
  }, 1600);
});
