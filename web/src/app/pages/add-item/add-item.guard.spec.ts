import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Router, RouterStateSnapshot, provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';

import { addItemDeactivateGuard } from './add-item.guard';
import type { AddItemPageComponent } from './add-item-page.component';
import { environment } from '../../config/environment';
import { authInterceptor } from '../../services/auth.interceptor';
import { AuthService } from '../../services/auth.service';

describe('addItemDeactivateGuard', () => {
    describe('destinations', () => {
        let signedIn: boolean;
        let page: { canDeactivate: ReturnType<typeof vi.fn> };

        const guard = (url: string) =>
            TestBed.runInInjectionContext(() =>
                addItemDeactivateGuard(
                    page as unknown as AddItemPageComponent,
                    {} as never,
                    {} as never,
                    { url } as RouterStateSnapshot,
                ),
            );

        beforeEach(() => {
            signedIn = true;
            page = { canDeactivate: vi.fn(() => false) }; // saving
            TestBed.configureTestingModule({
                providers: [
                    provideRouter([]),
                    { provide: AuthService, useValue: { isAuthenticated: () => signedIn } },
                ],
            });
        });

        it('lets the sign-in redirect through once the session is cleared', () => {
            signedIn = false;
            expect(guard('/login?redirectTo=%2Fitems%2Fadd')).toBe(true);
            expect(guard('/login')).toBe(true);
            expect(page.canDeactivate).not.toHaveBeenCalled();
        });

        it('keeps blocking the login route while still signed in', () => {
            expect(guard('/login?redirectTo=%2Fitems%2Fadd')).toBe(false);
            expect(page.canDeactivate).toHaveBeenCalledOnce();
        });

        it('keeps blocking other and lookalike routes after sign-out', () => {
            signedIn = false;
            for (const url of [
                '/',
                '/shelves',
                '/loginx',
                '/login/extra',
                '/items/login',
                '/login;next=1',
                '/login(sidebar:menu)',
                '/LOGIN',
            ]) {
                expect(guard(url), url).toBe(false);
            }
        });

        it('leaves navigation alone when nothing is saving', () => {
            page.canDeactivate.mockReturnValue(true);
            expect(guard('/')).toBe(true);
            expect(guard('/login')).toBe(true);
        });
    });

    describe('with the real router and auth flow', () => {
        @Component({ template: '' })
        class SavingPageStub {
            saving = true;
            canDeactivate(): boolean {
                return !this.saving;
            }
        }
        @Component({ template: '' })
        class BlankStub {}

        let router: Router;
        let http: HttpTestingController;
        let auth: AuthService;

        beforeEach(async () => {
            TestBed.configureTestingModule({
                providers: [
                    provideRouter([
                        {
                            path: 'items/add',
                            component: SavingPageStub,
                            canDeactivate: [addItemDeactivateGuard],
                        },
                        { path: 'login', component: BlankStub },
                        { path: '', component: BlankStub },
                        { path: '**', component: BlankStub },
                    ]),
                    provideHttpClient(withInterceptors([authInterceptor])),
                    provideHttpClientTesting(),
                ],
            });
            router = TestBed.inject(Router);
            http = TestBed.inject(HttpTestingController);
            auth = TestBed.inject(AuthService);

            auth.ensureSession().subscribe();
            http.expectOne(`${environment.apiUrl}/session`).flush({ authenticated: true });
            // The harness renders a router outlet, so the guard sees the live page.
            const harness = await RouterTestingHarness.create();
            const page = await harness.navigateByUrl('/items/add', SavingPageStub);
            expect(page.saving).toBe(true);
        });

        afterEach(() => http.verify());

        it('blocks ordinary navigation while saving', async () => {
            expect(await router.navigateByUrl('/')).toBe(false);
            expect(await router.navigateByUrl('/login')).toBe(false);
            expect(router.url).toBe('/items/add');
        });

        it('follows the 401 redirect to sign-in while saving', async () => {
            TestBed.inject(HttpClient)
                .get(`${environment.apiUrl}/items`)
                .subscribe({ error: () => undefined });
            http.expectOne(`${environment.apiUrl}/items`).flush(
                { error: 'authentication required' },
                { status: 401, statusText: 'Unauthorized' },
            );

            await vi.waitFor(() => expect(router.url).toBe('/login?redirectTo=%2Fitems%2Fadd'));
        });

        it('follows the logout redirect while saving', async () => {
            auth.logout().subscribe(() => void router.navigate(['/login']));
            http.expectOne(`${environment.apiUrl}/session`).flush(null, {
                status: 204,
                statusText: 'No Content',
            });

            await vi.waitFor(() => expect(router.url).toBe('/login'));
        });

        it('still blocks other routes after sign-out while saving', async () => {
            auth.markUnauthenticated();
            expect(await router.navigateByUrl('/')).toBe(false);
            expect(await router.navigateByUrl('/loginx')).toBe(false);
            expect(router.url).toBe('/items/add');
        });
    });
});
