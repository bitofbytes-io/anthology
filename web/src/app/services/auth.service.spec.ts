import { TestBed } from '@angular/core/testing';
import { HttpClientTestingModule, HttpTestingController } from '@angular/common/http/testing';

import { AuthService } from './auth.service';
import { environment } from '../config/environment';

describe(AuthService.name, () => {
    let service: AuthService;
    let httpMock: HttpTestingController;
    let redirectSpy: ReturnType<typeof vi.spyOn>;

    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [HttpClientTestingModule],
        });

        service = TestBed.inject(AuthService);
        httpMock = TestBed.inject(HttpTestingController);

        redirectSpy = vi.spyOn(service as any, 'redirectTo');
    });

    afterEach(() => {
        httpMock.verify();
    });

    function setSessionState(value: { authenticated: boolean }) {
        (
            service as unknown as {
                sessionState: {
                    next: (value: { authenticated: boolean }) => void;
                };
            }
        ).sessionState.next(value);
    }

    it('redirects to Google OAuth without redirectTo when none is provided', () => {
        service.loginWithGoogle();

        expect(redirectSpy).toHaveBeenCalledWith(`${environment.apiUrl}/auth/google`);
    });

    it('redirects to Google OAuth with redirectTo when provided', () => {
        service.loginWithGoogle('/items');

        expect(redirectSpy).toHaveBeenCalledWith(
            `${environment.apiUrl}/auth/google?redirectTo=%2Fitems`,
        );
    });

    it('omits redirectTo when it points back to login', () => {
        service.loginWithGoogle('/login');

        expect(redirectSpy).toHaveBeenCalledWith(`${environment.apiUrl}/auth/google`);
    });

    it('returns cached session state without making a request', () => {
        setSessionState({
            authenticated: true,
        });

        let value: boolean | undefined;
        service.ensureSession().subscribe((result) => {
            value = result;
        });

        expect(value).toBe(true);
        httpMock.expectNone(`${environment.apiUrl}/session`);
    });

    it('fetches session status when not cached', () => {
        let value: boolean | undefined;
        service.ensureSession().subscribe((result) => {
            value = result;
        });

        const req = httpMock.expectOne(`${environment.apiUrl}/session`);
        expect(req.request.withCredentials).toBe(true);
        req.flush({
            authenticated: true,
            user: { id: '1', email: 'user@example.com', name: 'User', avatarUrl: 'avatar.png' },
        });

        expect(value).toBe(true);
        expect(service.isAuthenticated()).toBe(true);
        expect(service.getUser()?.email).toBe('user@example.com');
    });

    it('treats 401 as unauthenticated when checking session', () => {
        let value: boolean | undefined;
        service.ensureSession().subscribe((result) => {
            value = result;
        });

        const req = httpMock.expectOne(`${environment.apiUrl}/session`);
        req.flush({ message: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });

        expect(value).toBe(false);
        expect(service.isAuthenticated()).toBe(false);
    });

    it('clears session state on logout success', () => {
        setSessionState({
            authenticated: true,
        });

        service.logout().subscribe();

        const req = httpMock.expectOne(`${environment.apiUrl}/session`);
        expect(req.request.method).toBe('DELETE');
        expect(req.request.withCredentials).toBe(true);
        req.flush(null);

        expect(service.isAuthenticated()).toBe(false);
        expect(service.getUser()).toBeNull();
    });

    it('does not throw on 401 logout responses', () => {
        let error: unknown;
        service.logout().subscribe({
            error: (err) => {
                error = err;
            },
        });

        const req = httpMock.expectOne(`${environment.apiUrl}/session`);
        req.flush({ message: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });

        expect(error).toBeUndefined();
        expect(service.isAuthenticated()).toBe(false);
    });
});
