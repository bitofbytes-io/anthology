import {
    Component,
    DestroyRef,
    inject,
    OnInit,
    signal,
    ChangeDetectionStrategy,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';
import { ActivatedRoute, RouterModule } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

import { AuthService } from '../../services/auth.service';

@Component({
    selector: 'app-login-page',
    standalone: true,
    imports: [RouterModule, MatButtonModule, MatCardModule, MatIconModule],
    templateUrl: './login-page.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './login-page.component.scss',
})
export class LoginPageComponent implements OnInit {
    private readonly authService = inject(AuthService);
    private readonly route = inject(ActivatedRoute);
    private readonly destroyRef = inject(DestroyRef);

    readonly submitting = signal(false);
    readonly errorMessage = signal<string | null>(null);

    ngOnInit(): void {
        // Check for OAuth callback errors
        const error = this.route.snapshot.queryParamMap.get('error');
        const message = this.route.snapshot.queryParamMap.get('message');
        if (error) {
            this.errorMessage.set(message || this.getErrorMessage(error));
        }
    }

    loginWithGoogle(): void {
        this.submitting.set(true);
        this.errorMessage.set(null);
        const redirectTo = this.route.snapshot.queryParamMap.get('redirectTo');
        this.authService.loginWithGoogle(redirectTo || undefined);
    }

    clearSession(): void {
        this.submitting.set(true);
        this.errorMessage.set(null);
        this.authService
            .logout()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => {
                    this.submitting.set(false);
                },
                error: () => {
                    this.submitting.set(false);
                    this.errorMessage.set('We could not clear your session.');
                },
            });
    }

    private getErrorMessage(code: string): string {
        switch (code) {
            case 'access_denied':
                return 'Your account is not authorized to access this application.';
            case 'email_not_verified':
                return 'Please verify your Google email address first.';
            case 'invalid_request':
                return 'The login request was invalid. Please try again.';
            case 'exchange_error':
                return 'Failed to complete authentication. Please try again.';
            case 'internal_error':
                return 'An internal error occurred. Please try again later.';
            default:
                return 'An error occurred during login. Please try again.';
        }
    }
}
