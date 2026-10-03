import {
    ChangeDetectionStrategy,
    Component,
    computed,
    EventEmitter,
    Input,
    Output,
    signal,
    Signal,
} from '@angular/core';

import { LetterHistogram } from '../../models';

const ALPHABET: string[] = [
    'A',
    'B',
    'C',
    'D',
    'E',
    'F',
    'G',
    'H',
    'I',
    'J',
    'K',
    'L',
    'M',
    'N',
    'O',
    'P',
    'Q',
    'R',
    'S',
    'T',
    'U',
    'V',
    'W',
    'X',
    'Y',
    'Z',
    '#',
];

@Component({
    selector: 'app-alpha-rail',
    standalone: true,
    imports: [],
    templateUrl: './alpha-rail.component.html',
    styleUrl: './alpha-rail.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AlphaRailComponent {
    @Input() histogram: Signal<LetterHistogram> = signal({});
    @Input() activeLetter: Signal<string | null> = signal(null);

    @Output() readonly letterSelected = new EventEmitter<string>();

    readonly visibleLetters = computed(() => {
        const hist = this.histogram();
        return ALPHABET.filter((letter) => (hist[letter] ?? 0) > 0);
    });

    selectLetter(letter: string): void {
        this.letterSelected.emit(letter);
    }

    isActive(letter: string): boolean {
        return this.activeLetter() === letter;
    }

    getCount(letter: string): number {
        return this.histogram()[letter] ?? 0;
    }

    handleKeydown(event: KeyboardEvent, letters: string[], index: number): void {
        let targetIndex: number | null = null;

        if (event.key === 'ArrowDown') {
            event.preventDefault();
            targetIndex = index < letters.length - 1 ? index + 1 : 0;
        } else if (event.key === 'ArrowUp') {
            event.preventDefault();
            targetIndex = index > 0 ? index - 1 : letters.length - 1;
        } else if (event.key === 'Home') {
            event.preventDefault();
            targetIndex = 0;
        } else if (event.key === 'End') {
            event.preventDefault();
            targetIndex = letters.length - 1;
        }

        if (targetIndex !== null) {
            const buttons = (event.currentTarget as HTMLElement)
                .closest('.alpha-rail')
                ?.querySelectorAll<HTMLButtonElement>('.rail-letter:not(.rail-letter--all)');
            buttons?.[targetIndex]?.focus();
        }
    }
}
