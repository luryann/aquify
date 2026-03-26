# STEP 99D — Admin UI Enhancements

**Status:** COMPLETE
**Date:** 2026-03-23

## Tasks

### 1. Resizable Sidebar ✅
- Drag handle on right edge of sidebar
- Min 160px, max 380px
- Persist width to localStorage (`aquify-sidebar-width`)
- Keep collapse toggle (52px when collapsed)
- Disable CSS transition during drag
- Update `--sidebar-w` CSS variable for other components

### 2. Season Management Page ✅
- Route: `/seasons`
- Nav: System section (owner/admin)
- Current season: name, start date, end date, status
- Next season: name, start date, end date
- Season goals textarea
- Save/edit per card

### 3. Training Page ✅
- Route: `/training`
- Nav: new "Training" section (owner/admin/coach)
- Tab 1 — Schedule: add workout (date, group, type, description, yardage), list workouts
- Tab 2 — Stats: weekly yards per group, stroke breakdown bars, attendance

### 4. Visual Consistency (globals.css) ✅
- Ensure .page-header pattern is defined
- Table row height 40px
- Consistent 32px content padding

### 5. New Meet File Uploads ✅
- Basic Info step gets two upload zones
- "Meet Package Files" — PDF/images (attachments)
- "Meet Entry File" — .cl2/.sd3/.zip/.hy3, triggers auto-parse for sessions

### 6. Fixed Session Builder ✅
- Labeled card layout with proper field labels above inputs
- No more unlabeled 5-column grid

## Files Modified/Created
- `admin/components/layout/Sidebar.tsx`
- `admin/app/(admin)/seasons/page.tsx` (NEW)
- `admin/app/(admin)/training/page.tsx` (NEW)
- `admin/lib/roles.ts`
- `admin/app/(admin)/meets/new/page.tsx`
