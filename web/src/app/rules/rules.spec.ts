import { TestBed } from '@angular/core/testing';

import { stubDialog } from '../testing/dialog-stub';
import { Rules } from './rules';

function setup() {
  stubDialog();
  const fixture = TestBed.createComponent(Rules);
  let closed = 0;
  fixture.componentInstance.closed.subscribe(() => closed++);
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  const dialog = host.querySelector('dialog') as HTMLDialogElement;
  const open = () => {
    fixture.componentRef.setInput('open', true);
    fixture.detectChanges();
  };
  const button = (text: string) =>
    Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent?.includes(text) || b.getAttribute('aria-label') === text,
    ) as HTMLButtonElement;
  return { host, dialog, open, button, closed: () => closed };
}

describe('Rules', () => {
  it('arranca cerrado y se abre cuando se pide', () => {
    const { dialog, open } = setup();
    expect(dialog.open).toBe(false);

    open();

    expect(dialog.open).toBe(true);
  });

  it('se cierra con la ×, con "Entendido" y con un clic en el fondo', () => {
    for (const close of ['Cerrar', 'Entendido', 'fondo']) {
      const { dialog, open, button, closed } = setup();
      open();

      if (close === 'fondo') dialog.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      else button(close).click();

      expect(dialog.open).toBe(false);
      expect(closed()).toBe(1);
      TestBed.resetTestingModule();
    }
  });

  it('un clic dentro del contenido no lo cierra', () => {
    const { dialog, open, host, closed } = setup();
    open();

    (host.querySelector('.lead') as HTMLElement).click();

    expect(dialog.open).toBe(true);
    expect(closed()).toBe(0);
  });

  it('explica el tablero, las tres formas de ganar y el empate', () => {
    const { host } = setup();
    const text = host.textContent ?? '';

    expect(text).toContain('8 columnas y 7 filas');
    expect(text).toContain('56 casillas');
    expect(Array.from(host.querySelectorAll('figcaption'), (f) => f.textContent?.trim())).toEqual([
      'Horizontal',
      'Vertical',
      'Diagonal',
    ]);
    // Cada ejemplo dibuja exactamente cuatro fichas en línea.
    for (const mini of Array.from(host.querySelectorAll('.mini'))) {
      expect(mini.querySelectorAll('.mini-dot--win').length).toBe(4);
    }
  });
});
