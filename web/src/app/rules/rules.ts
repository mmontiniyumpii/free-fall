import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  effect,
  input,
  output,
  viewChild,
} from '@angular/core';

type Dot = 'win' | 'empty';

/** Mini tableros de 4×4 para ilustrar cada forma de ganar (fila 0 arriba: es solo un dibujo). */
const EXAMPLES: readonly { label: string; dots: Dot[][] }[] = [
  { label: 'Horizontal', dots: grid((row) => (row === 3 ? [0, 1, 2, 3] : [])) },
  { label: 'Vertical', dots: grid(() => [1]) },
  { label: 'Diagonal', dots: grid((row) => [row]) },
];

function grid(winsIn: (row: number) => number[]): Dot[][] {
  return Array.from({ length: 4 }, (_, row) =>
    Array.from({ length: 4 }, (_, column) => (winsIn(row).includes(column) ? 'win' : 'empty')),
  );
}

/**
 * Reglas del juego en un <dialog> nativo: el navegador atrapa el foco, cierra
 * con Esc y oscurece el fondo. Solo texto: las reglas las aplica el bot.
 */
@Component({
  selector: 'app-rules',
  templateUrl: './rules.html',
  styleUrl: './rules.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Rules {
  readonly open = input(false);
  /** Dispositivo táctil: las instrucciones hablan de tocar, no de mouse ni teclas. */
  readonly touch = input(false);
  /** Se cerró por la ×, "Entendido", Esc o un clic afuera. */
  readonly closed = output<void>();

  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');
  protected readonly examples = EXAMPLES;

  constructor() {
    // Sincroniza el input con el estado real del <dialog>.
    effect(() => {
      const dialog = this.dialog().nativeElement;
      if (this.open() && !dialog.open) dialog.showModal();
      if (!this.open() && dialog.open) dialog.close();
    });
  }

  protected close(): void {
    this.dialog().nativeElement.close();
  }

  /** Un clic en el fondo oscuro llega al propio <dialog>; uno en el contenido, a sus hijos. */
  protected closeOnBackdrop(event: MouseEvent): void {
    if (event.target === this.dialog().nativeElement) this.close();
  }
}
