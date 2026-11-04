import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  effect,
  input,
  output,
  viewChild,
} from '@angular/core';

/**
 * Confirmación en un <dialog> nativo. Cancelar es la opción segura: tiene el
 * foco al abrir, y Esc o un clic afuera también cancelan.
 */
@Component({
  selector: 'app-confirm',
  templateUrl: './confirm.html',
  styleUrl: './confirm.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Confirm {
  readonly open = input(false);
  readonly heading = input.required<string>();
  readonly message = input.required<string>();
  readonly confirmLabel = input.required<string>();
  readonly cancelLabel = input.required<string>();

  readonly confirmed = output<void>();
  readonly cancelled = output<void>();

  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');
  private choice: 'confirm' | 'cancel' = 'cancel';

  constructor() {
    effect(() => {
      const dialog = this.dialog().nativeElement;
      if (this.open() && !dialog.open) {
        this.choice = 'cancel';
        dialog.showModal();
      }
      if (!this.open() && dialog.open) dialog.close();
    });
  }

  protected answer(choice: 'confirm' | 'cancel'): void {
    this.choice = choice;
    this.dialog().nativeElement.close();
  }

  /** Esc, clic afuera o un botón: el <dialog> se cierra y avisa una sola vez. */
  protected onClose(): void {
    if (this.choice === 'confirm') this.confirmed.emit();
    else this.cancelled.emit();
  }

  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog().nativeElement) this.answer('cancel');
  }
}
