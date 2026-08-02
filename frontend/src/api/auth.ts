// Thin re-export wrapper around the generated Wails bindings, so call sites
// in screens read like contracts/wails-bindings.md's conceptual "Auth.*"
// namespace even though the Go side binds flat method names on App (see
// app.go's naming note).
import {
    AuthGetStatus,
    AuthSignIn,
    AuthSignOut,
} from '../../wailsjs/go/main/App';
import type { events } from '../../wailsjs/go/models';

export type AuthStatus = events.AuthStatus;

export const GetStatus = AuthGetStatus;
export const SignIn = AuthSignIn;
export const SignOut = AuthSignOut;
