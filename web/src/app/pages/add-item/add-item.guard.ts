import { inject } from '@angular/core';
import { CanDeactivateFn, PRIMARY_OUTLET, Router } from '@angular/router';

import { AuthService } from '../../services/auth.service';
import type { AddItemPageComponent } from './add-item-page.component';

/**
 * Keeps the Add Item page open while a CSV import is saving, except for the
 * redirect to sign-in that follows a logout or an expired session: once the
 * session is cleared, the user must be able to reach the login page.
 */
export const addItemDeactivateGuard: CanDeactivateFn<AddItemPageComponent> = (
    component,
    _currentRoute,
    _currentState,
    nextState,
) => {
    if (nextState && isSignedOutLoginRedirect(nextState.url)) {
        return true;
    }
    return component?.canDeactivate() ?? true;
};

/**
 * True when the session has already been cleared (logout and the 401
 * interceptor both do that first) and url is exactly the login route, with
 * any query parameters.
 */
function isSignedOutLoginRedirect(url: string): boolean {
    const tree = inject(Router).parseUrl(url);
    const outlets = Object.keys(tree.root.children);
    const segments = tree.root.children[PRIMARY_OUTLET]?.segments ?? [];
    const isLoginRoute =
        outlets.length === 1 &&
        segments.length === 1 &&
        segments[0].path === 'login' &&
        Object.keys(segments[0].parameters).length === 0;
    return isLoginRoute && !inject(AuthService).isAuthenticated();
}
