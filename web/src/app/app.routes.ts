import { Routes } from '@angular/router';

import { addItemDeactivateGuard } from './pages/add-item/add-item.guard';
import { authGuard } from './services/auth.guard';

export const routes: Routes = [
    {
        path: '',
        canActivate: [authGuard],
        loadComponent: () =>
            import('./components/app-shell/app-shell.component').then((m) => m.AppShellComponent),
        children: [
            {
                path: '',
                loadComponent: () =>
                    import('./pages/items/items-page.component').then((m) => m.ItemsPageComponent),
            },
            {
                path: 'shelves',
                loadComponent: () =>
                    import('./pages/shelves/shelves-page/shelves-page.component').then(
                        (m) => m.ShelvesPageComponent,
                    ),
            },
            {
                path: 'shelves/add',
                loadComponent: () =>
                    import('./pages/shelves/add-shelf-page/add-shelf-page.component').then(
                        (m) => m.AddShelfPageComponent,
                    ),
            },
            {
                path: 'shelves/:id',
                loadComponent: () =>
                    import('./pages/shelves/shelf-detail-page/shelf-detail-page.component').then(
                        (m) => m.ShelfDetailPageComponent,
                    ),
            },
            {
                path: 'items/add',
                canDeactivate: [addItemDeactivateGuard],
                loadComponent: () =>
                    import('./pages/add-item/add-item-page.component').then(
                        (m) => m.AddItemPageComponent,
                    ),
            },
            {
                path: 'items/:id/edit',
                loadComponent: () =>
                    import('./pages/edit-item/edit-item-page.component').then(
                        (m) => m.EditItemPageComponent,
                    ),
            },
            {
                path: 'series',
                loadComponent: () =>
                    import('./pages/series/series-detail-page.component').then(
                        (m) => m.SeriesDetailPageComponent,
                    ),
            },
        ],
    },
    {
        path: 'login',
        loadComponent: () =>
            import('./pages/login/login-page.component').then((m) => m.LoginPageComponent),
    },
    { path: '**', redirectTo: '' },
];
