import { provideHttpClient } from '@angular/common/http';
import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';

import { environment } from '../environments/environment';
import { FakeGameApiService } from './game/fake-game-api.service';
import { GameApi } from './game/game-api';
import { GameApiService } from './game/game-api.service';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideHttpClient(),
    { provide: GameApi, useClass: environment.useFakeApi ? FakeGameApiService : GameApiService },
  ],
};
