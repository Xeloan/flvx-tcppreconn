# 070 - Mobile Tunnel Entry Select

Fix the tunnel create/edit multi-select dropdown so the 入口节点 selector opens reliably on mobile browsers inside the modal.

## Checklist

- [x] Inspect the tunnel modal and shared multi-select implementation
- [x] Move the shared multi-select popup out of clipping containers while preserving selection behavior
- [x] Validate the frontend build/lint flow after the UI fix
- [x] Revert unrelated lint-only edits before finalizing
