import { Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterModule } from '@angular/router';

import { ImportOutcome, OutcomeStatus } from '../csv-import-review';

const STATUS_LABELS: Record<OutcomeStatus, string> = {
    added: 'Added',
    skipped: 'Skipped',
    failed: 'Failed',
    interrupted: 'Stopped while saving',
    unprocessed: 'Not imported',
    unknown: 'Unknown',
};

/** Shows what happened to every previewed row after an import. */
@Component({
    selector: 'app-csv-import-result',
    standalone: true,
    imports: [MatButtonModule, MatIconModule, RouterModule],
    templateUrl: './import-result.component.html',
    styleUrl: './import-result.component.scss',
})
export class CsvImportResultComponent {
    readonly outcome = input.required<ImportOutcome>();
    readonly importAnother = output<void>();

    readonly statusLabels = STATUS_LABELS;

    /** Count tiles, always listing the four main outcomes and any unusual ones. */
    readonly tiles = computed(() => {
        const counts = this.outcome().counts;
        const statuses: OutcomeStatus[] = ['added', 'skipped', 'failed', 'unprocessed'];
        if (counts.interrupted > 0) {
            statuses.splice(3, 0, 'interrupted');
        }
        if (counts.unknown > 0) {
            statuses.push('unknown');
        }
        return statuses.map((status) => ({ status, count: counts[status] }));
    });

    /**
     * Selected rows the server skipped because a matching item was in the
     * library when the import ran, although the preview did not see one.
     */
    readonly skippedAtImport = computed(
        () => this.outcome().rows.filter((row) => row.sent && row.status === 'skipped').length,
    );

    /** True when the request ran out of time before every selected row was handled. */
    readonly stoppedEarly = computed(() =>
        this.outcome().rows.some(
            (row) => row.sent && (row.status === 'interrupted' || row.status === 'unprocessed'),
        ),
    );
}
