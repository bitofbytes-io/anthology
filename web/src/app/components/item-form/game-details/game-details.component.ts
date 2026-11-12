import { Component, Input, ChangeDetectionStrategy } from '@angular/core';
import { FormGroup, ReactiveFormsModule } from '@angular/forms';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';

@Component({
    selector: 'app-game-details',
    standalone: true,
    imports: [MatFormFieldModule, MatInputModule, ReactiveFormsModule],
    templateUrl: './game-details.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './game-details.component.scss',
})
export class GameDetailsComponent {
    @Input({ required: true }) form!: FormGroup;
}
