
// Script for compiling build behavior. It is built in the build plug-in and cannot be modified currently.
import path from 'path'

function requireCachedOhosPackage(packageName: string) {
    const hvigorSegment = `${path.sep}@ohos${path.sep}hvigor${path.sep}`;
    const hvigorModule = Object.keys(require.cache)
        .map((it) => path.normalize(it))
        .find((it) => it.includes(hvigorSegment));
    if (!hvigorModule) {
        return require(packageName);
    }
    const workspaceNodeModules = hvigorModule.split(hvigorSegment)[0];
    return require(path.join(workspaceNodeModules, '@ohos', packageName.split('/')[1]));
}

const { hapTasks } = requireCachedOhosPackage('@ohos/hvigor-ohos-plugin');
export default {
    system: hapTasks,  /* Built-in plugin of Hvigor. It cannot be modified. */
    plugins: []        /* Custom plugin to extend the functionality of Hvigor. */
}
