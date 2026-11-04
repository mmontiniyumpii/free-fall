import { TestBed } from '@angular/core/testing';

import { stubDialog } from '../testing/dialog-stub';
import { Confirm } from './confirm';

function setup() {
  stubDialog();
  const fixture = TestBed.createComponent(Confirm);
  fixture.componentRef.setInput('heading', '¿Volver al inicio?');
  fixture.componentRef.setInput('message', 'La partida en curso se pierde.');
  fixture.componentRef.setInput('confirmLabel', 'Volver al inicio');
  fixture.componentRef.setInput('cancelLabel', 'Seguir jugando');
  const events: string[] = [];
  fixture.componentInstance.confirmed.subscribe(() => events.push('confirmed'));
  fixture.componentInstance.cancelled.subscribe(() => events.push('cancelled'));
  fixture.componentRef.setInput('open', true);
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  const dialog = host.querySelector('dialog') as HTMLDialogElement;
  const button = (text: string) =>
    Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.trim() === text) as HTMLButtonElement;
  return { host, dialog, button, events };
}

describe('Confirm', () => {
  it('muestra la pregunta y la advertencia', () => {
    const { host, dialog } = setup();

    expect(dialog.open).toBe(true);
    expect(host.querySelector('.title')?.textContent).toContain('¿Volver al inicio?');
    expect(host.querySelector('.message')?.textContent).toContain('La partida en curso se pierde.');
  });

  it('confirmar avisa una sola vez y cierra', () => {
    const { dialog, button, events } = setup();

    button('Volver al inicio').click();

    expect(dialog.open).toBe(false);
    expect(events).toEqual(['confirmed']);
  });

  it('cancelar, Esc o clic afuera cancelan', () => {
    for (const how of ['botón', 'esc', 'fondo']) {
      const { dialog, button, events } = setup();

      if (how === 'botón') button('Seguir jugando').click();
      if (how === 'esc') dialog.close();
      if (how === 'fondo') dialog.dispatchEvent(new MouseEvent('click', { bubbles: true }));

      expect(events).toEqual(['cancelled']);
      TestBed.resetTestingModule();
    }
  });

  it('la opción segura (seguir jugando) tiene el foco al abrir', () => {
    const { button } = setup();

    expect(button('Seguir jugando').hasAttribute('autofocus')).toBe(true);
  });
});
