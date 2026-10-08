import { CanDeactivateFn } from '@angular/router';

import type { AddItemPageComponent } from './add-item-page.component';

/** Keeps the Add Item page open while a CSV import is saving. */
export const addItemDeactivateGuard: CanDeactivateFn<AddItemPageComponent> = (component) =>
    component?.canDeactivate() ?? true;
