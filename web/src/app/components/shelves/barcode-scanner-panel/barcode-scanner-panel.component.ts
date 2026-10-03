import {
    Component,
    DestroyRef,
    ElementRef,
    EventEmitter,
    inject,
    Input,
    NgZone,
    OnChanges,
    Output,
    SimpleChanges,
    ViewChild,
    computed,
} from '@angular/core';

import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';

import {
    BarcodeScannerService,
    BarcodeScanResult,
} from '../../../services/barcode-scanner.service';

@Component({
    selector: 'app-barcode-scanner-panel',
    standalone: true,
    imports: [MatProgressSpinnerModule],
    templateUrl: './barcode-scanner-panel.component.html',
    styleUrl: './barcode-scanner-panel.component.scss',
})
export class BarcodeScannerPanelComponent implements OnChanges {
    private readonly barcodeScanner = inject(BarcodeScannerService);
    private readonly ngZone = inject(NgZone);
    private readonly destroyRef = inject(DestroyRef);

    @ViewChild('scanVideo') scanVideo?: ElementRef<HTMLVideoElement>;
    @ViewChild('scannerSection') scannerSection?: ElementRef<HTMLDivElement>;

    @Input() active = false;
    @Input() instructions = 'Scan ISBN barcodes to automatically add and place items in this slot.';
    @Output() barcodeScanned = new EventEmitter<string>();

    readonly scannerSupported = computed(() => this.barcodeScanner.scannerSupported());
    readonly scannerActive = computed(() => this.barcodeScanner.scannerActive());
    readonly scannerReady = computed(() => this.barcodeScanner.scannerReady());
    readonly scannerStatus = computed(() => this.barcodeScanner.scannerStatus());
    readonly scannerError = computed(() => this.barcodeScanner.scannerError());
    readonly scannerHint = computed(() => this.barcodeScanner.scannerHint());
    readonly scannerProcessing = computed(() => this.barcodeScanner.scannerProcessing());
    readonly scannerFlash = computed(() => this.barcodeScanner.scannerFlash());

    constructor() {
        this.destroyRef.onDestroy(() => {
            this.stopScanner();
        });
    }

    ngOnChanges(changes: SimpleChanges): void {
        if (changes['active']) {
            if (this.active) {
                setTimeout(() => {
                    this.scrollIntoView();
                    void this.startScanner();
                }, 100);
            } else {
                this.stopScanner();
            }
        }
    }

    async startScanner(): Promise<void> {
        const video = this.scanVideo?.nativeElement;
        if (!video) {
            return;
        }

        await this.barcodeScanner.startScanner(video, (result: BarcodeScanResult) => {
            this.ngZone.run(() => {
                this.barcodeScanned.emit(result.rawValue);
            });
        });
    }

    stopScanner(): void {
        this.barcodeScanner.stopScanner();
    }

    reportScanComplete(): void {
        this.barcodeScanner.reportScanComplete();
    }

    private scrollIntoView(): void {
        this.scannerSection?.nativeElement.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
}
