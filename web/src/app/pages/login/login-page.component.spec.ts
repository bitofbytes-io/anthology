import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { of, throwError } from 'rxjs';

import { LoginPageComponent } from './login-page.component';
import { AuthService } from '../../services/auth.service';

describe(LoginPageComponent.name, () => {
    let authServiceSpy: {
        loginWithGoogle: ReturnType<typeof vi.fn>;
        logout: ReturnType<typeof vi.fn>;
    };

    beforeEach(async () => {
        authServiceSpy = {
            loginWithGoogle: vi.fn().mockName('AuthService.loginWithGoogle'),
            logout: vi.fn().mockName('AuthService.logout'),
        };

        await TestBed.configureTestingModule({
            imports: [LoginPageComponent],
            providers: [
                { provide: AuthService, useValue: authServiceSpy },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: { queryParamMap: convertToParamMap({ redirectTo: '/items' }) },
                    },
                },
            ],
        }).compileComponents();
    });

    function createComponent() {
        const fixture = TestBed.createComponent(LoginPageComponent);
        fixture.detectChanges();
        return fixture;
    }

    it('calls loginWithGoogle with redirectTo when clicking Google button', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        component.loginWithGoogle();

        expect(authServiceSpy.loginWithGoogle).toHaveBeenCalledWith('/items');
        expect(component.submitting()).toBe(true);
    });

    it('calls loginWithGoogle without redirectTo when none is provided', async () => {
        await TestBed.resetTestingModule();
        await TestBed.configureTestingModule({
            imports: [LoginPageComponent],
            providers: [
                { provide: AuthService, useValue: authServiceSpy },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: { queryParamMap: convertToParamMap({}) },
                    },
                },
            ],
        }).compileComponents();

        const fixture = TestBed.createComponent(LoginPageComponent);
        fixture.detectChanges();
        const component = fixture.componentInstance;

        component.loginWithGoogle();

        expect(authServiceSpy.loginWithGoogle).toHaveBeenCalledWith(undefined);
        expect(component.submitting()).toBe(true);
    });

    it('shows error message when query params contain error', async () => {
        // Reconfigure with error params
        await TestBed.resetTestingModule();
        await TestBed.configureTestingModule({
            imports: [LoginPageComponent],
            providers: [
                { provide: AuthService, useValue: authServiceSpy },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            queryParamMap: convertToParamMap({
                                error: 'access_denied',
                                message: 'Your account is not authorized.',
                            }),
                        },
                    },
                },
            ],
        }).compileComponents();

        const fixture = TestBed.createComponent(LoginPageComponent);
        fixture.detectChanges();
        const component = fixture.componentInstance;

        expect(component.errorMessage()).toBe('Your account is not authorized.');
    });

    it('shows default error message when error code has no custom message', async () => {
        await TestBed.resetTestingModule();
        await TestBed.configureTestingModule({
            imports: [LoginPageComponent],
            providers: [
                { provide: AuthService, useValue: authServiceSpy },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            queryParamMap: convertToParamMap({
                                error: 'unknown_error',
                            }),
                        },
                    },
                },
            ],
        }).compileComponents();

        const fixture = TestBed.createComponent(LoginPageComponent);
        fixture.detectChanges();
        const component = fixture.componentInstance;

        expect(component.errorMessage()).toContain('An error occurred');
    });

    it('clears the session when clearSession is called', () => {
        authServiceSpy.logout.mockReturnValue(of(void 0));
        const fixture = createComponent();
        const component = fixture.componentInstance;

        component.clearSession();

        expect(authServiceSpy.logout).toHaveBeenCalled();
        expect(component.submitting()).toBe(false);
    });

    it('shows error when logout fails', () => {
        authServiceSpy.logout.mockReturnValue(throwError(() => new Error('Network error')));
        const fixture = createComponent();
        const component = fixture.componentInstance;

        component.clearSession();

        expect(component.errorMessage()).toContain('could not clear');
        expect(component.submitting()).toBe(false);
    });
});
