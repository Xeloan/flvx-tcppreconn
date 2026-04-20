# PLAN: Fix Listbox unclickable due to pointer-events bug in Dropdown

## Tasks
- [x] Add `pointer-events-auto` to `select.tsx` portal wrapper
- [x] Add native and react stopPropagation in `select.tsx`
- [x] Catch `onInteractOutside` in `ModalContent` inside `modal.tsx` to handle `role=listbox`
